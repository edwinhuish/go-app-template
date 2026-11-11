package repository_test

// 本文件守住 keyword 的语义：用户输入按「字面包含」匹配，% 与 _ 不是通配符。
//
// 为什么值得测：漏了转义不会报错，只会让「搜 %」返回整张表——列表接口按身份收窄
// 范围这件事在 service 层，那里假设 keyword 只是一个过滤词；一个 % 就能让这条假设
// 落空，而界面上只是「搜不到东西」和「搜出全部」的差别，没人会当成 bug 报。

import (
	"context"
	"slices"
	"testing"

	"example.com/app/internal/model"
	"example.com/app/internal/repository"
)

func TestNoteKeywordIsLiteral(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	for _, title := range []string{"alpha", "100% done", "under_score", "bang!"} {
		if err := f.notes.Create(ctx, &model.Note{Title: title, Content: "n"}); err != nil {
			t.Fatalf("创建 %q: %v", title, err)
		}
	}

	cases := []struct {
		keyword string
		want    []string
	}{
		{"%", []string{"100% done"}},    // 通配符当普通字符：只命中标题里真的带 % 的那条
		{"_", []string{"under_score"}},  // 同上；未转义时 _ 会匹配任意单字符
		{"!", []string{"bang!"}},        // 转义字符自己也得当字面量，否则 ! 会吃掉后面的字符
		{"100%", []string{"100% done"}}, // 与普通字符连用同样按字面
		{"alp", []string{"alpha"}},      // 正常子串照旧命中
		{"nosuch", nil},
	}

	for _, c := range cases {
		list, total, err := f.notes.Paginate(ctx,
			repository.Pagination{Page: 1, PageSize: 10},
			repository.NoteFilter{Keyword: c.keyword},
		)
		if err != nil {
			t.Fatalf("keyword=%q 查询: %v", c.keyword, err)
		}

		got := make([]string, 0, len(list))
		for _, n := range list {
			got = append(got, n.Title)
		}
		if total != int64(len(c.want)) || !slices.Equal(got, c.want) {
			t.Errorf("keyword=%q 命中 %v（total=%d），期望 %v", c.keyword, got, total, c.want)
		}
	}
}
