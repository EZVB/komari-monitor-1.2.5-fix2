package accounts

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/utils"
)

const constantSalt = "06Wm4Jv1Hkxx"

func CheckPassword(username, passwd string) (userUUID string, success bool) {
	db := dbcore.GetDBInstance()
	var user models.User
	if result := db.Where("username = ?", username).First(&user); result.Error != nil {
		return "", false
	}
	if hashPasswd(passwd) != user.Passwd {
		return "", false
	}
	return user.UUID, true
}

func ForceResetPassword(username, passwd string) error {
	db := dbcore.GetDBInstance()
	result := db.Model(&models.User{}).Where("username = ?", username).Update("passwd", hashPasswd(passwd))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func hashPasswd(passwd string) string {
	hash := sha256.New()
	hash.Write([]byte(passwd + constantSalt))
	return base64.StdEncoding.EncodeToString(hash.Sum(nil))
}

func CreateAccount(username, passwd string) (models.User, error) {
	db := dbcore.GetDBInstance()
	user := models.User{
		UUID:     uuid.New().String(),
		Username: username,
		Passwd:   hashPasswd(passwd),
	}
	if err := db.Create(&user).Error; err != nil {
		return models.User{}, err
	}
	return user, nil
}

func DeleteAccountByUsername(username string) error {
	return dbcore.GetDBInstance().Where("username = ?", username).Delete(&models.User{}).Error
}

func CreateDefaultAdminAccount() (username, passwd string, err error) {
	username = os.Getenv("ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}
	passwd = os.Getenv("ADMIN_PASSWORD")
	if passwd == "" {
		passwd = utils.GeneratePassword()
	}

	now := models.FromTime(time.Now())
	user := models.User{
		UUID:      uuid.New().String(),
		Username:  username,
		Passwd:    hashPasswd(passwd),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := dbcore.GetDBInstance().Create(&user).Error; err != nil {
		return "", "", err
	}
	return username, passwd, nil
}

func GetUserByUUID(userUUID string) (models.User, error) {
	var user models.User
	if err := dbcore.GetDBInstance().Where("uuid = ?", userUUID).First(&user).Error; err != nil {
		return models.User{}, err
	}
	return user, nil
}

func UpdateUser(userUUID string, name, password *string) error {
	db := dbcore.GetDBInstance()
	var existingUser models.User
	if result := db.Where("uuid = ?", userUUID).First(&existingUser); result.Error != nil {
		return fmt.Errorf("user not found: %s", userUUID)
	}

	updates := make(map[string]interface{})
	if name != nil {
		updates["username"] = *name
	}
	if password != nil {
		updates["passwd"] = hashPasswd(*password)
	}
	updates["updated_at"] = time.Now()
	if err := db.Model(&models.User{}).Where("uuid = ?", userUUID).Updates(updates).Error; err != nil {
		return err
	}
	if password != nil {
		DeleteAllSessions()
	}
	return nil
}
