package web

import (
	"context"
	"fmt"

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
var _ RegistryStore = (*MySQLStore)(nil)

func (s *MySQLStore) table(name string) string {
	return s.db.NamingStrategy.TableName(name)
}

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

type registryRow struct {
	ID             int32
	Name           string
	Address        string
	Namespace      string
	CredentialID   int32
	CredentialName string
}

func (s *MySQLStore) ListRegistries(ctx context.Context) ([]Registry, error) {
	registry := s.table("registry")
	credential := s.table("credential")
	var rows []registryRow
	err := s.db.WithContext(ctx).
		Table(fmt.Sprintf("%s AS registry", registry)).
		Select("registry.id, registry.name, registry.address, registry.namespace, registry.credential_id, credential.name AS credential_name").
		Joins(fmt.Sprintf("LEFT JOIN %s AS credential ON credential.id = registry.credential_id", credential)).
		Order("registry.id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Registry, len(rows))
	for i, row := range rows {
		out[i] = Registry{
			ID:             row.ID,
			Name:           row.Name,
			Address:        row.Address,
			Namespace:      row.Namespace,
			CredentialID:   row.CredentialID,
			CredentialName: row.CredentialName,
		}
	}
	return out, nil
}

func (s *MySQLStore) GetRegistry(ctx context.Context, id int32) (Registry, error) {
	registry := s.table("registry")
	credential := s.table("credential")
	var row registryRow
	err := s.db.WithContext(ctx).
		Table(fmt.Sprintf("%s AS registry", registry)).
		Select("registry.id, registry.name, registry.address, registry.namespace, registry.credential_id, credential.name AS credential_name").
		Joins(fmt.Sprintf("LEFT JOIN %s AS credential ON credential.id = registry.credential_id", credential)).
		Where("registry.id = ?", id).
		Take(&row).Error
	if err != nil {
		return Registry{}, err
	}
	return Registry{
		ID:             row.ID,
		Name:           row.Name,
		Address:        row.Address,
		Namespace:      row.Namespace,
		CredentialID:   row.CredentialID,
		CredentialName: row.CredentialName,
	}, nil
}

func (s *MySQLStore) CreateRegistry(ctx context.Context, r Registry) error {
	row := model.Registry{
		Name:         r.Name,
		Address:      r.Address,
		Namespace:    r.Namespace,
		CredentialID: r.CredentialID,
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *MySQLStore) UpdateRegistry(ctx context.Context, r Registry) error {
	return s.db.WithContext(ctx).Model(&model.Registry{}).Where("id = ?", r.ID).Updates(map[string]interface{}{
		"name":          r.Name,
		"address":       r.Address,
		"namespace":     r.Namespace,
		"credential_id": r.CredentialID,
	}).Error
}

func (s *MySQLStore) DeleteRegistry(ctx context.Context, id int32) error {
	var taskCount int64
	if err := s.db.WithContext(ctx).Model(&model.SyncTask{}).Where("registry_id = ?", id).Count(&taskCount).Error; err != nil {
		return err
	}
	if err := model.CanDeleteRegistry(int(taskCount)); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&model.Registry{}, id).Error
}
