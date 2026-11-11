package repository

import "strings"

// likeEscape 是 LIKE 模式里的转义字符。
//
// 用 '!' 而不是反斜杠：反斜杠在 SQL 字符串字面量与 Go 字符串里各要再转义一层，
// 而 MySQL 与 Postgres（standard_conforming_strings=on）对 '\\' 的解释并不一致，
// 同一个写法总有一边报错或行为不同。'!' 在 MySQL / Postgres / SQLite 里都是普通字符。
const likeEscape = '!'

// likeContainsCondition 拼出「某列包含关键词」的 WHERE 条件，参数用 likeContains 生成。
//
// 列的取值与转义字符都写在这里，二者必须成套出现：ESCAPE 子句漏掉时，转义过的
// 模式会退化成「反斜杠是普通字符」，输入里的 \! 之类会静默匹配不到。
// SQLite 的 LIKE 没有默认转义字符（MySQL / Postgres 默认是反斜杠），所以 ESCAPE
// 必须显式写出来，不能指望驱动的默认值。
func likeContainsCondition(column string) string {
	return column + " LIKE ? ESCAPE '" + string(likeEscape) + "'"
}

// likeContains 把用户输入包成「包含」模式，并把 LIKE 的通配符转义成普通字符。
//
// 不转义的后果不是「查不到」，而是「查到的比要的多」：输入一个 % 就等于取消过滤，
// 把整张表翻出来。对本包的调用点来说，这还意味着一个 % 能越过按身份收窄的范围
// （service 虽然会覆盖 owner_id，但它依赖「keyword 只是一个过滤词」这个前提）。
//
// 两条用这条路查询的既有事实，写在这里省得下一个人重新踩：
//   - 模式以 % 开头，B-tree 索引一律用不上，代价随表行数线性增长。只适合小表，
//     或者像本包那样同时还带着等值条件（owner_id）先把范围收窄。
//   - 大小写敏感度随驱动不同：MySQL 默认按列的 collation（通常 ci，不敏感），
//     Postgres 的 LIKE 区分大小写（要 ILIKE），SQLite 只对 ASCII 不区分。
//     需要跨库一致时不要依赖这一层，应在写入时归一化到单独一列。
func likeContains(keyword string) string {
	esc := string(likeEscape)
	// Replacer 单趟扫描、匹配不重叠，所以「先加倍转义字符」不会把它再处理一次。
	r := strings.NewReplacer(esc, esc+esc, "%", esc+"%", "_", esc+"_")
	return "%" + r.Replace(keyword) + "%"
}
