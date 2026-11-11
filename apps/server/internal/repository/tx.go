package repository

import (
	"context"

	"gorm.io/gorm"
)

// TxManager 把一次跨仓储的写入收进同一个事务。
//
// 用法：
//
//	err := txs.Transaction(ctx, func(ctx context.Context) error {
//		// 这里所有的仓储调用都自动落在同一个事务上
//		return nil
//	})
//
// 事务连接放在 ctx 里而不是作为参数往下传：服务层的业务方法动辄调用三四个仓储，
// 参数传递要改一整条调用链，而只要有一个仓储拿的是池子连接，
// 这个流程就在「提交到一半」的地方断开了——那种 bug 只在失败路径上出现，
// 平时跑得比谁都正常。
type TxManager interface {
	// Transaction 在同一个事务里执行 fn。fn 返回错误即回滚。
	// 已经在事务里时不再开新事务。
	Transaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type txKey struct{}

// txFrom 取 ctx 里的事务连接，没有就返回 nil。
func txFrom(ctx context.Context) *gorm.DB {
	tx, _ := ctx.Value(txKey{}).(*gorm.DB)
	return tx
}

// dbFor 取这次操作该用的连接：ctx 里带着事务就用事务，否则用给定连接。
//
// 不嵌入 Base 的仓储（比如自己发号的）也走它：否则它会另开一条连接，
// sqlite 上就会被外层事务的写锁卡住直到超时。
func dbFor(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx := txFrom(ctx); tx != nil {
		return tx.WithContext(ctx)
	}
	return db.WithContext(ctx)
}

type txManager struct {
	db *gorm.DB
}

// NewTxManager 构造事务管理器。
func NewTxManager(db *gorm.DB) TxManager { return &txManager{db: db} }

func (m *txManager) Transaction(ctx context.Context, fn func(context.Context) error) error {
	if txFrom(ctx) != nil {
		// 已经在别人的事务里：不再开第二层。
		// 用新连接等于把「外层回滚」变成「内层已经提交」，那是最难查的一类脏数据。
		return fn(ctx)
	}
	// 事务本身不吞错误：errs 的分类要靠原样的 error 类型走到 handler。
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}
