package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"frameworklab/internal/labkit"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schemas/*.json
var schemaFS embed.FS

type OrderV1 struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Amount     int    `json:"amount"`
	Status     string `json:"status"`
}

type OrderV2 struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Amount     int    `json:"amount"`
	Status     string `json:"status"`
	Currency   string `json:"currency"`
}

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, OrderV1{ID: r.PathValue("id"), CustomerID: "c-1", Amount: 199, Status: "created"})
	})
	mux.HandleFunc("GET /v2/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, OrderV2{ID: r.PathValue("id"), CustomerID: "c-1", Amount: 199, Status: "created", Currency: "CNY"})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func loadSchema(name string) (*jsonschema.Schema, error) {
	data, err := schemaFS.ReadFile("schemas/" + name)
	if err != nil {
		return nil, err
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	location := "https://frameworklab.local/schemas/" + name
	if err := compiler.AddResource(location, document); err != nil {
		return nil, err
	}
	return compiler.Compile(location)
}

func validate(schema *jsonschema.Schema, payload []byte) error {
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return schema.Validate(document)
}

func get(handler http.Handler, path string) []byte {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Body.Bytes()
}

func main() {
	lab := labkit.New("Day 29", "契约测试：JSON Schema、版本兼容与消费者保护")
	lab.Section("框架简介：JSON Schema")
	lab.Infof("类型：契约与验证规范")
	lab.Infof("是什么：用可执行 schema 描述 JSON 的形状、必填字段、枚举和兼容边界")
	lab.Infof("解决什么问题：接口字段会随版本漂移；仅靠 Go struct 无法证明实际响应满足消费者合同")
	lab.Infof("核心抽象：schema、required、additionalProperties、enum、CI 中验证响应和事件 payload")
	lab.Infof("采用情况：JSON Schema 是跨语言标准；santhosh-tekuri/jsonschema 是 Go 中成熟的实现之一")
	lab.Infof("适合：对外 API、服务间事件、需要消费者驱动契约和版本兼容检查的团队")
	lab.Infof("慎用：schema 不能替代领域不变量；严格模式可能拒绝新增字段；要先明确版本兼容策略")
	lab.Infof("对照：OpenAPI 描述接口，Protobuf 提供二进制契约，Pact 做消费者驱动契约测试")
	handler := Handler()
	v1Schema, err := loadSchema("order.v1.json")
	if err != nil {
		lab.Check(false, "load v1 schema: %v", err)
		lab.Done()
		return
	}
	v2Schema, err := loadSchema("order.v2.json")
	if err != nil {
		lab.Check(false, "load v2 schema: %v", err)
		lab.Done()
		return
	}

	lab.Section("场景 A：响应满足自己的版本契约")
	v1Body := get(handler, "/v1/orders/o-1")
	v2Body := get(handler, "/v2/orders/o-1")
	lab.Infof("v1=%s", strings.TrimSpace(string(v1Body)))
	lab.Infof("v2=%s", strings.TrimSpace(string(v2Body)))
	lab.Check(validate(v1Schema, v1Body) == nil, "v1 响应通过 v1 schema")
	lab.Check(validate(v2Schema, v2Body) == nil, "v2 响应通过 v2 schema")

	lab.Section("场景 B：新增字段对旧消费者可能是破坏性变更")
	v1Error := validate(v1Schema, v2Body)
	v2Error := validate(v2Schema, v1Body)
	lab.Infof("v2 against v1 schema: %v", v1Error)
	lab.Infof("v1 against v2 schema: %v", v2Error)
	lab.Check(v1Error != nil, "additionalProperties=false 的旧契约会拒绝 v2 新字段")
	lab.Check(v2Error != nil, "新契约要求 currency，旧响应同样不满足")

	lab.Section("场景 C：契约测试放在 CI 里")
	lab.Infof("把 schema 当作消费者合同；每次改 DTO 都运行 validate")
	lab.Infof("严格 schema 适合内部服务；对外 API 要明确 unknown-field 和版本兼容策略")
	lab.Infof("OpenAPI 负责描述接口，JSON Schema 负责验证响应形状，两者应同时纳入流水线")
	lab.Done()
}
