package main

import (
	"golab/internal/fsm"
	"golab/internal/labkit"
)

func build(retries int) *fsm.Flow {
	f := fsm.New()
	for _, id := range []string{"start", "extract", "clean", "join", "publish"} {
		max := 1
		if id == "clean" {
			max = retries
		}
		_ = f.AddNode(id, max)
	}
	for _, e := range [][2]string{
		{"start", "extract"},
		{"extract", "clean"},
		{"clean", "join"},
		{"join", "publish"},
	} {
		_ = f.AddEdge(e[0], e[1])
	}
	f.Init()
	return f
}

func runStage(lab *labkit.Lab, f *fsm.Flow, id string) {
	if err := f.Start(id); err != nil {
		lab.Failf("start %s: %v", id, err)
		return
	}
	if err := f.Succeed(id); err != nil {
		lab.Failf("succeed %s: %v", id, err)
		return
	}
	lab.Infof("node %-8s success (attempt=%d)", id, f.Attempts(id))
}

func printStates(lab *labkit.Lab, f *fsm.Flow) {
	for _, n := range f.Snapshot() {
		lab.Infof("%-8s state=%-8s attempts=%d/%d", n.ID, n.State, n.Attempts, n.MaxAttempts)
	}
}

func main() {
	lab := labkit.New("Day 09", "节点状态机：重试、跳过与流程终态")

	lab.Section("场景 A：全部成功")
	f := build(1)
	for _, id := range []string{"start", "extract", "clean", "join", "publish"} {
		runStage(lab, f, id)
	}
	printStates(lab, f)
	lab.Check(f.Final() == fsm.Success, "所有节点 success，流程终态 success")

	lab.Section("场景 B：clean 第一次失败，重试后成功")
	f2 := build(2)
	for _, id := range []string{"start", "extract"} {
		runStage(lab, f2, id)
	}
	_ = f2.Start("clean")
	retry, err := f2.Fail("clean")
	lab.Infof("clean fail #1 -> retrying=%v err=%v attempts=%d", retry, err, f2.Attempts("clean"))
	lab.Check(retry && f2.State("clean") == fsm.Ready, "未超过最大次数，节点回到 ready")
	runStage(lab, f2, "clean")
	for _, id := range []string{"join", "publish"} {
		runStage(lab, f2, id)
	}
	printStates(lab, f2)
	lab.Check(f2.Final() == fsm.Success, "重试后整条链路成功")

	lab.Section("场景 C：clean 重试耗尽，下游必须跳过")
	f3 := build(2)
	for _, id := range []string{"start", "extract"} {
		runStage(lab, f3, id)
	}
	_ = f3.Start("clean")
	_, _ = f3.Fail("clean")
	_ = f3.Start("clean")
	retry, err = f3.Fail("clean")
	lab.Infof("clean fail #2 -> retrying=%v err=%v", retry, err)
	lab.Check(!retry && f3.State("clean") == fsm.Failed, "重试耗尽后节点终态 failed")
	lab.Check(f3.State("join") == fsm.Skipped && f3.State("publish") == fsm.Skipped, "下游被递归标记 skipped")
	printStates(lab, f3)
	lab.Check(f3.Final() == fsm.Failed, "任一节点 failed，流程终态 failed")

	lab.Section("状态迁移日志（场景 C 最后 6 条）")
	transitions := f3.Transitions()
	start := len(transitions) - 6
	if start < 0 {
		start = 0
	}
	for _, t := range transitions[start:] {
		lab.Infof("%s", t)
	}

	lab.Section("场景 D：非法迁移必须被拒绝")
	err = f3.Start("publish")
	lab.Infof("start skipped publish -> %v", err)
	lab.Check(err != nil, "skipped 节点不能直接 start")

	lab.Section("Java → Go 对照")
	lab.Infof("enum + switch 状态机              ≈  typed string + 显式迁移函数")
	lab.Infof("synchronized 保护状态             ≈  Flow 内部 mutex；真正的跨进程状态仍需 DB CAS")
	lab.Infof("attempt / rerun / retryTimes      ≈  (nodeInstanceID, rerunID, retryTimes) 三元组")
	lab.Infof("流程终态                          ≈  all terminal + 任一 failed 则 failed")
	lab.Done()
}
