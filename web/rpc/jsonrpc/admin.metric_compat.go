package jsonrpc

import (
	"context"
	"strings"

	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/pkg/config"
	"github.com/komari-monitor/komari/pkg/rpc"
)

const (
	loadRetentionMetricName = "legacy.load_records"
	pingRetentionMetricName = "legacy.ping_records"
)

type legacyRetentionMetric struct {
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Type          string            `json:"type"`
	Unit          string            `json:"unit,omitempty"`
	RetentionDays int               `json:"retention_days"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

func init() {
	RegisterWithGroupAndMeta("listMetricDefinitions", rpc.RoleAdmin, adminListMetricDefinitions, &rpc.MethodMeta{
		Name:    "admin:listMetricDefinitions",
		Summary: "List legacy monitoring record retention settings",
		Returns: "MetricDefinition[]",
	})
	RegisterWithGroupAndMeta("updateMetricDefinition", rpc.RoleAdmin, adminUpdateMetricDefinition, &rpc.MethodMeta{
		Name:    "admin:updateMetricDefinition",
		Summary: "Update a legacy monitoring record retention setting",
		Returns: "MetricDefinition",
	})
}

func retentionHoursToDays(hours int) int {
	if hours <= 0 {
		return 0
	}
	return (hours + 23) / 24
}

func retentionMetric(name string, hours int) legacyRetentionMetric {
	metric := legacyRetentionMetric{
		Name:          name,
		Type:          "history",
		Unit:          "record",
		RetentionDays: retentionHoursToDays(hours),
	}

	switch name {
	case loadRetentionMetricName:
		metric.Description = "Server load and resource history"
		metric.Metadata = map[string]string{
			"display_name": `{"zh-CN":"负载监控数据","zh-TW":"負載監控資料","en":"Load monitoring data"}`,
		}
	case pingRetentionMetricName:
		metric.Description = "Ping latency and packet-loss history"
		metric.Metadata = map[string]string{
			"display_name": `{"zh-CN":"延迟监控数据","zh-TW":"延遲監控資料","en":"Ping monitoring data"}`,
		}
	}

	return metric
}

func adminListMetricDefinitions(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	loadHours, err := config.GetAs[int](config.RecordPreserveTimeKey, 720)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to get load record retention: "+err.Error(), nil)
	}
	pingHours, err := config.GetAs[int](config.PingRecordPreserveTimeKey, 24)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to get ping record retention: "+err.Error(), nil)
	}

	return []legacyRetentionMetric{
		retentionMetric(loadRetentionMetricName, loadHours),
		retentionMetric(pingRetentionMetricName, pingHours),
	}, nil
}

func adminUpdateMetricDefinition(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Name          string `json:"name"`
		RetentionDays int    `json:"retention_days"`
	}
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid metric retention parameters: "+err.Error(), nil)
	}
	params.Name = strings.TrimSpace(params.Name)
	if params.RetentionDays < 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "retention_days must be zero or greater", nil)
	}

	var key string
	switch params.Name {
	case loadRetentionMetricName:
		key = config.RecordPreserveTimeKey
	case pingRetentionMetricName:
		key = config.PingRecordPreserveTimeKey
	default:
		return nil, rpc.MakeError(rpc.InvalidParams, "Unknown metric definition: "+params.Name, nil)
	}

	hours := params.RetentionDays * 24
	if err := config.Set(key, hours); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to update metric retention: "+err.Error(), nil)
	}

	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "updated "+params.Name+" retention", "info")
	return retentionMetric(params.Name, hours), nil
}
