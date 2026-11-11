// Package dialect 是数据库方言名的唯一定义处。
//
// 同一个名字有两个来源：配置里的 database.driver 决定开哪条连接，
// gorm 的 Dialector.Name() 在连接建立后报出实际方言。两者取值相同，
// 所以字面量只在这里写一次——配置常量、连接装配、方言分支都引用本包，
// 新增驱动时不会出现「改了一处漏了另一处」。
package dialect

const (
	// Postgres 对应 gorm.io/driver/postgres。
	Postgres = "postgres"
	// MySQL 对应 gorm.io/driver/mysql。
	MySQL = "mysql"
	// SQLite 对应 github.com/glebarez/sqlite（纯 Go 实现，无需 cgo）。
	SQLite = "sqlite"
)
