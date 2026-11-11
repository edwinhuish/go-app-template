package handler

import (
	"time"

	"github.com/go-fuego/fuego"

	"example.com/app/internal/dto"
)

// Health 供负载均衡与容器健康检查使用，不参与鉴权。
//
// 只证明「进程还能接请求」；要证明依赖可用（数据库、缓存），
// 应另开一个 /readyz 并在那里 ping，避免健康检查把重启放大成雪崩。
func Health(c fuego.ContextNoBody) (dto.HealthStatus, error) {
	return dto.HealthStatus{
		Status: "ok",
		Time:   time.Now().Format(time.RFC3339),
	}, nil
}
