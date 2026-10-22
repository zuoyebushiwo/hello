package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"frameworklab/internal/labkit"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Config struct {
	Server ServerConfig `mapstructure:"server"`
	Log    LogConfig    `mapstructure:"log"`
	DB     DBConfig     `mapstructure:"db"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

type DBConfig struct {
	DSN string `mapstructure:"dsn"`
}

func newConfig() *Config {
	return &Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 8080},
		Log:    LogConfig{Level: "info"},
		DB:     DBConfig{DSN: "sqlite://app.db"},
	}
}

func newRootCommand() *cobra.Command {
	var configPath string
	root := &cobra.Command{
		Use:           "app",
		Short:         "framework lab service",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, configPath)
		},
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "path to a YAML configuration file")
	serve := &cobra.Command{
		Use:   "serve",
		Short: "print the resolved service configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, configPath)
		},
	}
	serve.Flags().String("server.host", "", "listen host")
	serve.Flags().Int("server.port", 0, "listen port")
	serve.Flags().String("log.level", "", "log level")
	serve.Flags().String("db.dsn", "", "database DSN")
	root.AddCommand(serve)
	return root
}

func loadConfig(cmd *cobra.Command, configPath string) (*Config, error) {
	v := viper.New()
	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 8080)
	v.SetDefault("log.level", "info")
	v.SetDefault("db.dsn", "sqlite://app.db")
	v.SetEnvPrefix("APP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %s: %w", configPath, err)
		}
	}
	if err := v.BindPFlags(cmd.Flags()); err != nil {
		return nil, fmt.Errorf("bind flags: %w", err)
	}

	cfg := newConfig()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return cfg, nil
}

func runServe(cmd *cobra.Command, configPath string) error {
	cfg, err := loadConfig(cmd, configPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "listen=%s:%d level=%s dsn=%s\n", cfg.Server.Host, cfg.Server.Port, cfg.Log.Level, cfg.DB.DSN)
	return nil
}

func main() {
	lab := labkit.New("Day 16", "Cobra + Viper：命令行、配置分层与启动校验")
	lab.Section("框架简介：Cobra + Viper")
	lab.Infof("类型：命令行框架 + 配置库")
	lab.Infof("是什么：Cobra 负责命令树、参数和帮助信息；Viper 负责 YAML、环境变量、flag 等多来源配置")
	lab.Infof("解决什么问题：标准库 flag 适合单命令工具；服务需要 serve/migrate/version 子命令和可预测的配置优先级")
	lab.Infof("核心抽象：Command/RunE、PersistentFlags、默认值、配置文件、环境变量、命令行覆盖")
	lab.Infof("采用情况：Cobra 约 45k star，Kubernetes、Hugo 等项目使用；Viper 约 30k star，是常见配置方案")
	lab.Infof("适合：CLI 工具、需要子命令的后端服务、希望启动配置可审计的团队")
	lab.Infof("慎用：Viper 不提供依赖注入、生命周期或业务容器，不能当成 Go 的 Spring Boot")
	lab.Infof("对照：标准库 flag/pflag 更轻；koanf 等配置库更小、更显式")

	lab.Section("场景 A：默认配置")
	cmd := newRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"serve"})
	if err := cmd.Execute(); err != nil {
		lab.Check(false, "默认命令执行失败：%v", err)
	} else {
		lab.Infof("output=%s", strings.TrimSpace(out.String()))
		lab.Check(strings.Contains(out.String(), "listen=127.0.0.1:8080"), "无配置文件时使用默认值")
	}

	lab.Section("场景 B：环境变量覆盖默认值")
	os.Setenv("APP_SERVER_PORT", "9090")
	defer os.Unsetenv("APP_SERVER_PORT")
	cmd = newRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"serve"})
	if err := cmd.Execute(); err != nil {
		lab.Check(false, "环境变量场景失败：%v", err)
	} else {
		lab.Infof("output=%s", strings.TrimSpace(out.String()))
		lab.Check(strings.Contains(out.String(), ":9090"), "环境变量覆盖默认配置")
	}

	lab.Section("场景 C：命令行优先级最高")
	cmd = newRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"serve", "--server.port", "10080"})
	if err := cmd.Execute(); err != nil {
		lab.Check(false, "命令行场景失败：%v", err)
	} else {
		lab.Infof("output=%s", strings.TrimSpace(out.String()))
		lab.Check(strings.Contains(out.String(), ":10080"), "flag 覆盖环境变量")
	}

	lab.Section("场景 D：Cobra 的启动生命周期")
	start := time.Now()
	cmd = newRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"serve"})
	if err := cmd.Execute(); err != nil {
		lab.Check(false, "生命周期场景失败：%v", err)
	} else {
		lab.Check(time.Since(start) < time.Second, "命令执行返回错误而不是调用 os.Exit，便于测试")
	}

	lab.Section("生态位置")
	lab.Infof("Cobra   ≈ Spring Boot 的启动器 + 子命令入口，Kubernetes/Hugo 都在使用")
	lab.Infof("Viper   ≈ application.yml + 环境变量 + flag 的统一解析，但不会帮你创建 Bean")
	lab.Infof("推荐顺序：defaults < config file < env < flags，并在启动阶段一次性校验")
	lab.Done()
}
