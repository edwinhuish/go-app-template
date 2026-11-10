package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"example.com/app/internal/pkg/errs"
)

// BodyLimit 给请求体设一个上限，超了直接 413。
//
// 为什么需要它：gin 的 `ShouldBindJSON` 会把整个请求体读进内存再解析，
// 而 Go 默认没有上限——一个几 GB 的 POST 足以把进程的内存吃光，
// 且这类请求只需要一个 curl 循环，不需要任何账号（登录接口也在保护范围内）。
//
// 业务上如果另有更小的上限（上传 10MB 之类），两层都要留：
// 业务上限决定「允许多大的文件」，这一层决定「允许多大的请求」。
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			// 提前用 Content-Length 拒一次：能省掉把数据读进来再丢掉。
			if c.Request.ContentLength > maxBytes {
				failTooLarge(c, maxBytes)
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

// HandleBodyTooLarge 把 MaxBytesReader 抛出的读错误翻成 413。
//
// 不翻的话它会在 handler 里变成「解析请求体失败」这类 400，
// 而 400 的意思是「你的请求写错了」——实际是「太大了」，两种提示对应的动作完全不同。
func HandleBodyTooLarge(c *gin.Context, err error) bool {
	var tooLarge *http.MaxBytesError
	if !errors.As(err, &tooLarge) {
		return false
	}
	failTooLarge(c, tooLarge.Limit)
	return true
}

func failTooLarge(c *gin.Context, limit int64) {
	msg := "请求体过大"
	if limit > 0 {
		msg += "（上限 " + humanSize(limit) + "）"
	}
	c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
		"code":    errs.CodeInvalidParam,
		"message": msg,
	})
}

func humanSize(n int64) string {
	const mb = 1 << 20
	if n >= mb {
		return strconv.FormatInt(n/mb, 10) + "MB"
	}
	return strconv.FormatInt(n/(1<<10), 10) + "KB"
}
