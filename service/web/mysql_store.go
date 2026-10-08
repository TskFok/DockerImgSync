package web

import (
	"context"

	"github.com/TskFok/DockerImgSync/app/model"
	"github.com/TskFok/DockerImgSync/utils/crypto"
	"gorm.io/gorm"
)

type MySQLStore struct {
	db  *gorm.DB
	key []byte
}

func NewMySQLStore(db *gorm.DB, key []byte) *MySQLStore {
	return &MySQLStore{db: db, key: key}
}

var _ CredentialStore = (*MySQLStore)(nil)

func (s *MySQLStore) ListCredentials(ctx context.Context) ([]Credential, error) {
	var rows []model.Credential
	if err := s.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Credential, len(rows))
	for i, row := range rows {
		out[i] = Credential{ID: row.ID, Name: row.Name, Username: row.Username}
	}
	return out, nil
}

func (s *MySQLStore) GetCredential(ctx context.Context, id int32) (Credential, error) {
	var row model.Credential
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return Credential{}, err
	}
	return Credential{ID: row.ID, Name: row.Name, Username: row.Username}, nil
}

func (s *MySQLStore) CreateCredential(ctx context.Context, c Credential) error {
	enc, err := crypto.Encrypt(s.key, c.Password)
	if err != nil {
		return err
	}
	row := model.Credential{
		Name:              c.Name,
		Username:          c.Username,
		PasswordEncrypted: enc,
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *MySQLStore) UpdateCredential(ctx context.Context, c Credential) error {
	updates := map[string]interface{}{
		"name":     c.Name,
		"username": c.Username,
	}
	if c.Password != "" {
		enc, err := crypto.Encrypt(s.key, c.Password)
		if err != nil {
			return err
		}
		updates["password_encrypted"] = enc
	}
	return s.db.WithContext(ctx).Model(&model.Credential{}).Where("id = ?", c.ID).Updates(updates).Error
}

func (s *MySQLStore) DeleteCredential(ctx context.Context, id int32) error {
	var registryCount int64
	if err := s.db.WithContext(ctx).Model(&model.Registry{}).Where("credential_id = ?", id).Count(&registryCount).Error; err != nil {
		return err
	}
	var taskCount int64
	if err := s.db.WithContext(ctx).Model(&model.SyncTask{}).Where("source_credential_id = ?", id).Count(&taskCount).Error; err != nil {
		return err
	}
	if err := model.CanDeleteCredential(int(registryCount), int(taskCount)); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&model.Credential{}, id).Error
}
