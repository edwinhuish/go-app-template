// Package service 是业务层。
//
// 它只依赖 repository 接口、cache 接口和 dto，不认识 HTTP 框架，也不写 SQL：
// 换 HTTP 框架、换数据库都不需要改这一层。
//
// 每个服务都遵循同一套写法：
//   - 对外暴露一个接口（便于替换与打桩）+ 一个 NewXxxService 构造函数；
//   - 可选依赖用 Option 传入（WithXxxLogger 之类），必填依赖走构造参数；
//   - 返回的错误一律是 pkg/errs 里的 *Error，handler 只负责翻译成 HTTP。
package service

// Option 是各服务构造参数的通用形状，具体服务用各自的别名类型。
//
// 只声明形状，不提供「未指定 logger 时退回 slog.Default()」这类公共默认值：
// 需要 logger 的服务自己写 `if s.log == nil`，不需要的服务就别留一个只赋值不读的字段。
// 共享一个 helper 的结果是每个服务都挂上 logger，而其中一半从来没打过日志。
type Option[T any] func(*T)
