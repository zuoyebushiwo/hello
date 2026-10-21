package main

import (
	"errors"
	"fmt"
	"sync"

	"golab/internal/di"
	"golab/internal/labkit"
)

type AppConfig struct{ Name string }

type FakeDB struct {
	DSN    string
	closed bool
}

type Repository struct{ DB *FakeDB }

type Service struct {
	Repo *Repository
	Cfg  AppConfig
}

func main() {
	lab := labkit.New("Day 13", "显式依赖装配：typed key、懒加载、逆序关闭")

	configKey := di.NewKey[AppConfig]("config")
	dbKey := di.NewKey[*FakeDB]("db")
	repoKey := di.NewKey[*Repository]("repository")
	serviceKey := di.NewKey[*Service]("service")

	var mu sync.Mutex
	var events []string
	record := func(s string) {
		mu.Lock()
		events = append(events, s)
		mu.Unlock()
	}
	snapshot := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), events...)
	}

	c := di.New()
	_ = di.Provide(c, configKey, func(*di.Container) (AppConfig, error) {
		record("build config")
		return AppConfig{Name: "dataflow"}, nil
	}, func(AppConfig) error { record("close config"); return nil })
	_ = di.Provide(c, dbKey, func(ctn *di.Container) (*FakeDB, error) {
		record("build db")
		cfg := di.MustGet(ctn, configKey)
		return &FakeDB{DSN: cfg.Name + "-dsn"}, nil
	}, func(db *FakeDB) error { record("close db"); db.closed = true; return nil })
	_ = di.Provide(c, repoKey, func(ctn *di.Container) (*Repository, error) {
		record("build repository")
		return &Repository{DB: di.MustGet(ctn, dbKey)}, nil
	}, func(*Repository) error { record("close repository"); return nil })
	_ = di.Provide(c, serviceKey, func(ctn *di.Container) (*Service, error) {
		record("build service")
		return &Service{Repo: di.MustGet(ctn, repoKey), Cfg: di.MustGet(ctn, configKey)}, nil
	}, func(*Service) error { record("close service"); return nil })

	lab.Section("场景 A：懒加载，Get 时才构建")
	lab.Infof("events before Get = %v", snapshot())
	svc, err := di.Get(c, serviceKey)
	lab.Infof("service = %+v err=%v", svc, err)
	lab.Infof("build order = %v", c.BuiltOrder())
	lab.Check(err == nil && svc.Repo.DB.DSN == "dataflow-dsn", "依赖链 Config -> DB -> Repo -> Service 自动解析")
	lab.Check(len(c.BuiltOrder()) == 4, "四个 bean 都只构建一次")

	lab.Section("场景 B：Close 按逆序释放")
	err = c.Close()
	lab.Infof("close error = %v", err)
	lab.Infof("events = %v", snapshot())
	lab.Check(err == nil, "没有关闭错误")
	lab.Check(fmt.Sprint(snapshot()) == "[build service build repository build db build config close service close repository close db close config]", "释放顺序与构建完成顺序相反")
	lab.Check(svc.Repo.DB.closed, "DB 连接被显式关闭")
	lab.Check(len(c.BuiltOrder()) == 4, "关闭后仍可审计构建顺序")

	lab.Section("场景 C：构建失败时，已构建的资源仍会被释放")
	c2 := di.New()
	db2Key := di.NewKey[*FakeDB]("db2")
	cfg2Key := di.NewKey[AppConfig]("config2")
	_ = di.Provide(c2, cfg2Key, func(*di.Container) (AppConfig, error) {
		record("c2 build config")
		return AppConfig{Name: "x"}, nil
	}, func(AppConfig) error { record("c2 close config"); return nil })
	_ = di.Provide(c2, db2Key, func(*di.Container) (*FakeDB, error) {
		record("c2 build db")
		return nil, errors.New("dial failed")
	}, nil)
	_, _ = di.Get(c2, cfg2Key)
	_, err = di.Get(c2, db2Key)
	lab.Infof("build error = %v", err)
	_ = c2.Close()
	lab.Check(err != nil, "构建失败会返回带依赖名的错误")
	lab.Infof("events after failed startup = %v", snapshot())
	lab.Check(fmt.Sprint(snapshot()) == "[build service build repository build db build config close service close repository close db close config c2 build config c2 build db c2 close config]", "失败启动只释放已成功构建的 bean")

	lab.Section("场景 D：循环依赖必须显式报错")
	c3 := di.New()
	aKey := di.NewKey[AppConfig]("a")
	bKey := di.NewKey[*FakeDB]("b")
	_ = di.Provide(c3, aKey, func(ctn *di.Container) (AppConfig, error) {
		_, e := di.Get(ctn, bKey)
		return AppConfig{}, e
	}, nil)
	_ = di.Provide(c3, bKey, func(ctn *di.Container) (*FakeDB, error) {
		_, e := di.Get(ctn, aKey)
		return nil, e
	}, nil)
	_, err = di.Get(c3, aKey)
	lab.Infof("cycle error = %v", err)
	lab.Check(err != nil, "循环依赖被 building 标记拦截")

	lab.Section("Java → Go 对照")
	lab.Infof("@Autowired / 包扫描             ≈  Provide + Get，全部显式可见")
	lab.Infof("Bean 名称字符串                  ≈  Key[T] 泛型 key，Get 返回值有类型")
	lab.Infof("@PreDestroy / DisposableBean    ≈  Close 逆序释放，errors.Join 汇总")
	lab.Infof("启动失败回滚                      ≈  只关闭已构建的 bean，不碰未构建的")
	lab.Done()
}
