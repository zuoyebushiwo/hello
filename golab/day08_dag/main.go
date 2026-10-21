package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golab/internal/graph"
	"golab/internal/labkit"
	"golab/internal/workerpool"
)

func build() *graph.Graph {
	g := graph.New()
	for _, id := range []string{"start", "extract", "clean_a", "clean_b", "join", "publish"} {
		g.AddNode(id)
	}
	for _, e := range [][2]string{
		{"start", "extract"},
		{"extract", "clean_a"},
		{"extract", "clean_b"},
		{"clean_a", "join"},
		{"clean_b", "join"},
		{"join", "publish"},
	} {
		_ = g.AddEdge(e[0], e[1])
	}
	return g
}

func execute(ctx context.Context, g *graph.Graph) (int64, error) {
	layers, err := g.Layers()
	if err != nil {
		return 0, err
	}
	pool := workerpool.New(ctx, 3, 16)
	defer pool.Close()
	var done atomic.Int64
	for i, layer := range layers {
		var wg sync.WaitGroup
		for range layer {
			wg.Add(1)
			err := pool.Submit(func(ctx context.Context) {
				defer wg.Done()
				if !labkit.Sleep(ctx, 10*time.Millisecond) {
					return
				}
				done.Add(1)
			})
			if err != nil {
				return done.Load(), err
			}
		}
		wg.Wait()
		fmt.Printf("  layer %d completed: %v\n", i, layer)
	}
	return done.Load(), nil
}

func main() {
	lab := labkit.New("Day 08", "DAG：校验、拓扑排序与并行层")
	g := build()

	lab.Section("场景 A：菱形依赖图")
	topo, err := g.TopoSort()
	lab.Infof("topo order = %v (err=%v)", topo, err)
	lab.Check(err == nil && len(topo) == 6, "拓扑排序覆盖所有节点")
	layers, err := g.Layers()
	lab.Infof("layers = %v (err=%v)", layers, err)
	lab.Check(err == nil && len(layers) == 5, "菱形图被拆成 5 个执行层")
	lab.Check(len(layers[2]) == 2, "clean_a 与 clean_b 可以并行")

	lab.Section("场景 B：按层执行，层内并行")
	n, err := execute(context.Background(), g)
	lab.Infof("executed nodes=%d err=%v", n, err)
	lab.Check(err == nil && n == 6, "6 个节点全部执行")

	lab.Section("场景 C：成环必须在提交前失败")
	bad := graph.New()
	for _, id := range []string{"a", "b", "c"} {
		bad.AddNode(id)
	}
	_ = bad.AddEdge("a", "b")
	_ = bad.AddEdge("b", "c")
	_ = bad.AddEdge("c", "a")
	err = bad.Validate()
	lab.Infof("cycle error = %v", err)
	lab.Check(err != nil, "DFS 三色标记检测到环")
	if _, err := bad.Layers(); err == nil {
		lab.Failf("Layers 应该在环图上返回错误")
	} else {
		lab.OKf("Layers 拒绝环图：%v", err)
	}

	lab.Section("场景 D：未知节点不能悄悄忽略")
	missing := graph.New()
	missing.AddNode("a")
	err = missing.AddEdge("a", "not-exist")
	lab.Infof("missing edge error = %v", err)
	lab.Check(err != nil, "建边时校验节点存在，避免运行期空指针")

	lab.Section("Java → Go 对照")
	lab.Infof("没有标准 DAG 库                  ≈  自己实现 Kahn / DFS，算法比框架更重要")
	lab.Infof("CompletableFuture.allOf          ≈  一层一个 WaitGroup，层间串行、层内并行")
	lab.Infof("内存中的图                        ≈  图只做校验与分层；运行时状态必须持久化")
	lab.Done()
}
