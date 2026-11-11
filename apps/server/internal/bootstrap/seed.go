package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"example.com/app/internal/model"
	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/repository"
	"example.com/app/internal/service"
)

// seed 写入开发所需的初始账号。
//
// 只在 database.seed=true 时执行，且以「账号已存在」为幂等条件：
// 反复重启不会重复插入，也不会把改过的密码重置回默认值。
//
// 生产环境必须在配置校验里关掉 seed（config.Validate 会直接拒绝启动）。
func (a *App) seed(
	ctx context.Context, users repository.UserRepository, auth service.AuthService,
) error {
	username := a.Config.App.SeedAdminUsername

	if _, err := users.FindByUsername(ctx, username); err == nil {
		a.Log.Info("初始账号已存在，跳过种子数据", "username", username)
		return nil
	} else if !errors.Is(err, errs.ErrNotFound) {
		return fmt.Errorf("查询初始账号: %w", err)
	}

	hash, err := auth.HashPassword(a.Config.App.SeedAdminPassword)
	if err != nil {
		return err
	}

	admin := &model.User{
		Username: username,
		Password: hash,
		Nickname: "管理员",
		Role:     model.RoleAdmin,
		Status:   model.EnableStatusOn,
	}
	if err := users.Create(ctx, admin); err != nil {
		return fmt.Errorf("创建初始账号: %w", err)
	}

	a.Log.Warn("已创建初始管理员账号，请尽快修改密码", "username", username)
	return nil
}
