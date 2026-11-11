package service

import (
	"context"
	"log/slog"

	"example.com/app/internal/dto"
	"example.com/app/internal/repository"
)

// UserService 提供账号目录的只读查询。
//
// 只读是有意的：账号的创建与改密属于「系统设置」，先由种子数据补齐；
// 等真的有运营需求时再单独开一个写接口，而不是让只读接口承担两种职责。
type UserService interface {
	List(ctx context.Context) ([]dto.UserItem, error)
}

// UserOption 是账号服务的可选依赖。
type UserOption = Option[userService]

// WithUserLogger 注入 logger。
func WithUserLogger(l *slog.Logger) UserOption {
	return func(s *userService) { s.log = l }
}

type userService struct {
	users repository.UserRepository
	log   *slog.Logger
}

// NewUserService 构造账号服务。
func NewUserService(users repository.UserRepository, opts ...UserOption) UserService {
	s := &userService{users: users, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *userService) List(ctx context.Context) ([]dto.UserItem, error) {
	list, err := s.users.List(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]dto.UserItem, len(list))
	for i, u := range list {
		items[i] = dto.UserItem{
			ID:       u.ID,
			Username: u.Username,
			Nickname: u.Nickname,
			Role:     u.Role,
			Status:   int8(u.Status),
		}
	}
	return items, nil
}
