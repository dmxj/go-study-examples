package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/dmxj/go-study-examples/rest-api/internal/model"
)

// UserRepository 定义用户的数据访问操作。
type UserRepository interface {
	Create(ctx context.Context, u *model.User) error
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	GetByID(ctx context.Context, id uint) (*model.User, error)
}

type gormUserRepo struct{ db *gorm.DB }

// NewUserRepository 创建基于 GORM 的实现。
func NewUserRepository(db *gorm.DB) UserRepository { return &gormUserRepo{db: db} }

func (r *gormUserRepo) Create(ctx context.Context, u *model.User) error {
	return translate(r.db.WithContext(ctx).Create(u).Error)
}

func (r *gormUserRepo) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error; err != nil {
		return nil, translate(err)
	}
	return &u, nil
}

func (r *gormUserRepo) GetByID(ctx context.Context, id uint) (*model.User, error) {
	var u model.User
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, translate(err)
	}
	return &u, nil
}
