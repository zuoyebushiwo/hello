package main

import (
	"errors"
	"net/url"

	"golab/internal/dsn"
	"golab/internal/labkit"
	"golab/internal/registry"
)

func main() {
	lab := labkit.New("Day 11", "扩展点：方言注册表与安全 DSN")

	lab.Section("场景 A：同一个字段，按目标方言转换")
	r := registry.Default()
	lab.Infof("registered pairs = %v", r.Pairs())
	cases := []struct {
		src, dst, in string
	}{
		{"mysql", "tidb", "VARCHAR(64)"},
		{"mysql", "kudu", "VARCHAR(64)"},
		{"mysql", "kudu", "BIGINT"},
		{"sqlserver", "tidb", "NVARCHAR(128)"},
	}
	for _, c := range cases {
		out, err := r.Convert(c.src, c.dst, c.in)
		lab.Infof("%-9s -> %-6s %-14s => %-12s err=%v", c.src, c.dst, c.in, out, err)
		if err != nil {
			lab.Failf("convert %s->%s: %v", c.src, c.dst, err)
		}
	}
	lab.Check(len(r.Pairs()) == 3, "三个转换器通过显式注册加入，不需要改调用方")

	lab.Section("场景 B：不支持的组合必须失败")
	_, err := r.Convert("oracle", "kudu", "NUMBER(10)")
	lab.Infof("oracle -> kudu error = %v", err)
	lab.Check(errors.Is(err, registry.ErrUnsupported), "未注册的方言组合返回可识别的错误")

	lab.Section("场景 C：重复注册要尽早失败")
	err = r.Register("mysql", "tidb", func(s string) string { return s })
	lab.Infof("duplicate register error = %v", err)
	lab.Check(err != nil, "重复注册不会被静默覆盖")

	lab.Section("场景 D：密码里有 @ : / 时，拼接 DSN 会坏")
	params := url.Values{
		"schema":       []string{"BLZY"},
		"autoCommit":   []string{"true"},
		"clobAsString": []string{"true"},
	}
	raw, err := dsn.Build("dm", "BL:ZY@user", "Poly@BLZY467\\:%end", "172.16.203.10", "5236", "BLZY", params)
	if err != nil {
		lab.Failf("build dsn: %v", err)
	}
	lab.Infof("dsn = %s", raw)
	u, err := dsn.Parse(raw)
	if err != nil {
		lab.Failf("parse dsn: %v", err)
	}
	pw, _ := u.User.Password()
	lab.Infof("parsed user=%q password=%q host=%q query=%v", u.User.Username(), pw, u.Host, u.Query())
	lab.Check(u.User.Username() == "BL:ZY@user", "用户名中的特殊字符被正确编码")
	lab.Check(pw == "Poly@BLZY467\\:%end", "密码中的特殊字符可完整还原")
	lab.Check(u.Query().Get("autoCommit") == "true", "query 参数结构化编码")

	lab.Section("Java → Go 对照")
	lab.Infof("ServiceLoader / Spring SPI       ≈  Registry + 显式 Register（调用方仍可编译期发现）")
	lab.Infof("策略模式 + Map<Pair, Converter>  ≈  这里就是最小实现")
	lab.Infof("new URL / UriComponentsBuilder   ≈  net/url.URL + url.UserPassword + net.JoinHostPort")
	lab.Infof("字符串拼 DSN                       ≈  密码含 @ 就会解析错，必须结构化构造")
	lab.Done()
}
