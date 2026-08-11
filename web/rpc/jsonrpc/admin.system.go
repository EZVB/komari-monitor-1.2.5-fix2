package jsonrpc

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/komari-monitor/komari/cmd/flags"
	"github.com/komari-monitor/komari/database/accounts"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/pkg/config"
	"github.com/komari-monitor/komari/pkg/rpc"
	"github.com/komari-monitor/komari/utils/cloudflared"
	"github.com/komari-monitor/komari/utils/geoip"
	"github.com/komari-monitor/komari/utils/messageSender"
)

// admin.system.go
// 系统/运维类 RPC2 方法（admin 命名空间）：日志、cloudflared、测试。

func init() {
	reg("getLogs", adminGetLogs, "Get audit logs (paged)")
	reg("getCloudflaredStatus", adminCloudflaredStatus, "Get cloudflared tunnel status")
	reg("startCloudflared", adminStartCloudflared, "Start cloudflared tunnel")
	reg("stopCloudflared", adminStopCloudflared, "Stop cloudflared tunnel")
	reg("removeCloudflaredToken", adminRemoveCloudflaredToken, "Remove cloudflared token")
	reg("testSendMessage", adminTestSendMessage, "Send a test notification")
	reg("testGeoip", adminTestGeoip, "Test GeoIP lookup")
	reg("getDatabaseSize", adminGetDatabaseSize, "Get the database file size on disk")
	reg("vacuumDatabase", adminVacuumDatabase, "Vacuum (compact) the SQLite database to reclaim disk space")

}

// databaseFileSize 统计 SQLite 数据库文件及其 WAL/SHM 附属文件占用的磁盘大小。
func databaseFileSize() int64 {
	if !flags.IsSQLite() {
		return 0
	}
	var total int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if info, err := os.Stat(flags.DatabaseFile + suffix); err == nil {
			total += info.Size()
		}
	}
	return total
}

func adminGetDatabaseSize(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	return map[string]any{
		"type": flags.NormalizeDatabaseType(flags.DatabaseType),
		"size": databaseFileSize(),
	}, nil
}

func adminVacuumDatabase(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	if !flags.IsSQLite() {
		return nil, rpc.MakeError(rpc.InvalidParams, "VACUUM is only supported for SQLite databases", nil)
	}

	before := databaseFileSize()

	db := dbcore.GetDBInstance()
	// 先做一次 WAL checkpoint，把 WAL 中的内容合并回主库，确保 VACUUM 能回收最多空间。
	db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	if err := db.Exec("VACUUM;").Error; err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to vacuum database: "+err.Error(), nil)
	}
	// VACUUM 后再次 checkpoint，回收 WAL 占用。
	db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")

	after := databaseFileSize()

	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "vacuumed database", "warn")

	return map[string]any{
		"before": before,
		"after":  after,
		"size":   after,
	}, nil
}

func adminGetLogs(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Limit string `json:"limit"`
		Page  string `json:"page"`
	}
	req.BindParams(&params)
	if params.Limit == "" {
		params.Limit = "100"
	}
	if params.Page == "" {
		params.Page = "1"
	}
	limitInt, err := strconv.Atoi(params.Limit)
	if err != nil || limitInt <= 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid limit: "+params.Limit, nil)
	}
	pageInt, err := strconv.Atoi(params.Page)
	if err != nil || pageInt <= 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid page: "+params.Page, nil)
	}
	db := dbcore.GetDBInstance()
	var logs []models.Log
	offset := (pageInt - 1) * limitInt
	var total int64
	if err := db.Model(&models.Log{}).Count(&total).Error; err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to count logs: "+err.Error(), nil)
	}
	if err := db.Order("time desc").Limit(limitInt).Offset(offset).Find(&logs).Error; err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to retrieve logs: "+err.Error(), nil)
	}
	return map[string]any{"logs": logs, "total": total}, nil
}

func adminCloudflaredStatus(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	return cloudflared.Status(), nil
}

func adminStartCloudflared(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Token string `json:"token"`
	}
	req.BindParams(&params)
	token := strings.TrimSpace(params.Token)
	if token != "" {
		if err := cloudflared.SaveToken(token); err != nil {
			return nil, rpc.MakeError(rpc.InternalError, "Failed to save Cloudflare Tunnel token: "+err.Error(), nil)
		}
	}
	if err := cloudflared.Start(token); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "started cloudflared tunnel", "warn")
	return cloudflared.Status(), nil
}

func adminStopCloudflared(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		CurrentPassword string `json:"current_password"`
	}
	req.BindParams(&params)

	actor, _ := auditActor(ctx)
	if actor == "" {
		return nil, rpc.MakeError(rpc.Unauthenticated, "Unauthorized.", nil)
	}
	user, err := accounts.GetUserByUUID(actor)
	if err != nil {
		return nil, rpc.MakeError(rpc.Unauthenticated, "Failed to verify current user", nil)
	}
	if strings.TrimSpace(params.CurrentPassword) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Current password is required", nil)
	}
	if _, ok := accounts.CheckPassword(user.Username, params.CurrentPassword); !ok {
		return nil, rpc.MakeError(rpc.Unauthenticated, "Current password is incorrect", nil)
	}

	if err := cloudflared.Stop(); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to stop cloudflared: "+err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "stopped cloudflared tunnel", "warn")
	return cloudflared.Status(), nil
}

func adminRemoveCloudflaredToken(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	if err := cloudflared.RemoveToken(); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Failed to remove Cloudflare Tunnel token: "+err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "removed cloudflared tunnel token", "warn")
	return cloudflared.Status(), nil
}

func adminTestSendMessage(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	err := messageSender.SendEvent(models.EventMessage{
		Event:   "Test",
		Message: "This is a test message from Komari.",
	})
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to send message: "+err.Error(), nil)
	}
	return nil, nil
}

func adminTestGeoip(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		IP string `json:"ip"`
	}
	req.BindParams(&params)
	ip := params.IP
	if ip == "" {
		if meta := rpc.MetaFromContext(ctx); meta != nil {
			ip = meta.RemoteIP
		}
	}
	cfg, err := config.GetAs[bool](config.GeoIpEnabledKey, false)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to get configuration: "+err.Error(), nil)
	}
	if !cfg {
		return nil, rpc.MakeError(rpc.InvalidParams, "GeoIP is not enabled in the configuration.", nil)
	}
	record, err := geoip.GetGeoInfo(net.ParseIP(ip))
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to get GeoIP record: "+err.Error(), nil)
	}
	return record, nil
}
