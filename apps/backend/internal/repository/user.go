package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"example.com/app/internal/model"
	"example.com/app/internal/pkg/errs"
)

// UserRepository 是账号的数据访问接口。
//
// 接口定义在消费者（service）所依赖的这一侧，实现细节不外露；
// 嵌入的 Base 已经提供 Create / FindByID / DeleteByID 等通用方法。
type UserRepository interface {
	Create(ctx context.Context, u *model.User) error
	FindByID(ctx context.Context, id uint) (*model.User, error)
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	List(ctx context.Context) ([]model.User, error)
}

type userRepository struct {
	*Base[model.User]
}

// NewUserRepository 构造账号仓储。
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{Base: NewBase[model.User](db)}
}

func (r *userRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := r.conn(ctx).Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("按用户名查询账号 %s: %w", username, err)
	}
	return &u, nil
}

func (r *userRepository) List(ctx context.Context) ([]model.User, error) {
	var list []model.User
	if err := r.conn(ctx).Order("id ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("查询账号列表: %w", err)
	}
	return list, nil
}
