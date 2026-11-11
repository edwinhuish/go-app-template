package server

import (
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// TestBindingSchemaCustomizer 守住 binding → schema 的映射。
//
// 这段逻辑没有运行时反馈：标签写错、映射改坏，接口照旧能跑，只是文档悄悄变错。
// 而「文档与实际不一致」比没有文档更费时间，所以用用例钉住。
func TestBindingSchemaCustomizer(t *testing.T) {
	type req struct {
		Title string `binding:"required,min=1,max=128"`
		Role  string `binding:"oneof=admin user"`
		Mail  string `binding:"email"`
		Page  int    `binding:"min=1,max=200"`
		Plain string
	}

	typ := reflect.TypeOf(req{})
	cases := []struct {
		field   string
		checkFn func(t *testing.T, got *openapi3.Schema)
	}{
		{
			field: "Title",
			checkFn: func(t *testing.T, got *openapi3.Schema) {
				if got.MinLength != 1 {
					t.Errorf("MinLength = %d, want 1", got.MinLength)
				}
				if got.MaxLength == nil || *got.MaxLength != 128 {
					t.Errorf("MaxLength = %v, want 128", got.MaxLength)
				}
			},
		},
		{
			field: "Role",
			checkFn: func(t *testing.T, got *openapi3.Schema) {
				if len(got.Enum) != 2 {
					t.Fatalf("Enum = %v, want 2 项", got.Enum)
				}
			},
		},
		{
			field: "Mail",
			checkFn: func(t *testing.T, got *openapi3.Schema) {
				if got.Format != "email" {
					t.Errorf("Format = %q, want email", got.Format)
				}
			},
		},
		{
			// 数字的 min/max 是取值而不是长度：同一个标签在两种类型上含义不同，
			// 弄反了会让文档写成「数字长度 1~200」这种没法执行的约束。
			field: "Page",
			checkFn: func(t *testing.T, got *openapi3.Schema) {
				if got.Min == nil || *got.Min != 1 {
					t.Errorf("Min = %v, want 1", got.Min)
				}
				if got.Max == nil || *got.Max != 200 {
					t.Errorf("Max = %v, want 200", got.Max)
				}
				if got.MaxLength != nil {
					t.Errorf("MaxLength 不该被设置，实际 %v", *got.MaxLength)
				}
			},
		},
		{
			// 没有 binding 标签的字段必须原样放过，不能被加上任何约束。
			field: "Plain",
			checkFn: func(t *testing.T, got *openapi3.Schema) {
				if got.MinLength != 0 || got.MaxLength != nil || got.Min != nil || got.Max != nil {
					t.Errorf("无标签字段被加了约束: %+v", got)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			field, ok := typ.FieldByName(c.field)
			if !ok {
				t.Fatalf("字段 %s 不存在", c.field)
			}
			schema := openapi3.NewStringSchema()
			if field.Type.Kind() == reflect.Int {
				schema = openapi3.NewIntegerSchema()
			}

			if err := bindingSchemaCustomizer(c.field, field.Type, field.Tag, schema); err != nil {
				t.Fatalf("customizer 返回错误: %v", err)
			}
			c.checkFn(t, schema)
		})
	}
}
