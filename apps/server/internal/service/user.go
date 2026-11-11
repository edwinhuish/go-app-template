package service

import (
	"context"

	"example.com/app/internal/dto"
	"example.com/app/internal/model"
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
// userService 没有 logger 字段：这一层只做查询与转换，没有需要记的运行时事件。
// 「先留一个字段以后用」的结果是它永远没被用到，却让人以为这里会写日志。
type userService struct {
	users repository.UserRepository
}

// NewUserService 构造账号服务。
func NewUserService(users repository.UserRepository) UserService {
	return &userService{users: users}
}

func (s *userService) List(ctx context.Context) ([]dto.UserItem, error) {
	list, err := s.users.List(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]dto.UserItem, len(list))
	for i := range list {
		items[i] = dto.UserItem{
			UserInfo: toUserInfo(&list[i]),
			Status:   int8(list[i].Status),
		}
	}
	return items, nil
}

// toUserInfo 是账号模型到对外形状的唯一映射，登录响应与账号列表都走这里。
//
// 放在 service 而不是 dto：dto 不 import model（对外形状不跟着持久化模型走），
// 互转是这一层的活。两个调用点各写一遍的话，加字段时总有一处漏改。
func toUserInfo(u *model.User) dto.UserInfo {
	return dto.UserInfo{
		ID:       u.ID,
		Username: u.Username,
		Nickname: u.Nickname,
		Role:     u.Role,
	}
}
