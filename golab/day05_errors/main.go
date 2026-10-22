package main

import (
	"errors"
	"fmt"
	"strings"

	"golab/internal/labkit"
)

var ErrNotFound = errors.New("not found")

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed: %s: %s", e.Field, e.Reason)
}

type RetryableError struct {
	Op  string
	Err error
}

func (e *RetryableError) Error() string   { return e.Op + ": " + e.Err.Error() }
func (e *RetryableError) Unwrap() error   { return e.Err }
func (e *RetryableError) Retryable() bool { return true }

type PermanentError struct {
	Op  string
	Err error
}

func (e *PermanentError) Error() string { return e.Op + ": " + e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

type nackError struct{ cause error }

func (e *nackError) Error() string { return "nack: " + e.cause.Error() }
func (e *nackError) Unwrap() error { return e.cause }

func loadUser(id string) error {
	switch id {
	case "404":
		return fmt.Errorf("load user %s: %w", id, ErrNotFound)
	case "bad":
		return fmt.Errorf("load user %s: %w", id, &ValidationError{Field: "id", Reason: "must be numeric"})
	case "flaky":
		return fmt.Errorf("load user %s: %w", id, &RetryableError{Op: "mysql query", Err: errors.New("connection reset")})
	default:
		return nil
	}
}

// classify uses the error chain, not the error text. This survives wrapping and
// localization, and it cannot be fooled by a message that merely contains the
// word "timeout".
func classify(err error) string {
	if err == nil {
		return "ok"
	}
	var validation *ValidationError
	if errors.As(err, &validation) {
		return "validation"
	}
	var retryable *RetryableError
	if errors.As(err, &retryable) {
		return "retryable"
	}
	var permanent *PermanentError
	if errors.As(err, &permanent) {
		return "permanent"
	}
	if errors.Is(err, ErrNotFound) {
		return "not_found"
	}
	return "unknown"
}

func processMessage() error {
	return fmt.Errorf("handle message: %w", &nackError{cause: errors.New("redis unavailable")})
}

func main() {
	lab := labkit.New("Day 05", "错误链：从字符串匹配到 errors.Is / errors.As")

	lab.Section("场景 A：同一个函数返回四类错误")
	for _, id := range []string{"404", "bad", "flaky", "ok"} {
		err := loadUser(id)
		lab.Infof("id=%-5s err=%-70v class=%s", id, err, classify(err))
	}
	lab.Check(classify(loadUser("404")) == "not_found", "sentinel error 用 errors.Is 判断")
	lab.Check(classify(loadUser("bad")) == "validation", "自定义错误用 errors.As 提取")
	lab.Check(classify(loadUser("flaky")) == "retryable", "可重试错误携带类型信息，不靠文案")

	lab.Section("场景 B：包装后直接类型断言失效")
	err := processMessage()
	_, direct := err.(*nackError)
	var target *nackError
	asOK := errors.As(err, &target)
	lab.Infof("err = %v", err)
	lab.Infof("err.(*nackError) -> %v", direct)
	lab.Infof("errors.As(err, &target) -> %v", asOK)
	lab.Check(!direct, "fmt.Errorf/%%w 包装后，直接类型断言失败")
	lab.Check(asOK, "errors.As 能沿 Unwrap 链找到底层类型")

	lab.Section("场景 C：errors.Join 同时保留多个错误")
	joined := errors.Join(
		fmt.Errorf("read config: %w", ErrNotFound),
		&ValidationError{Field: "port", Reason: "out of range"},
	)
	lab.Infof("joined = %v", joined)
	lab.Check(errors.Is(joined, ErrNotFound), "errors.Is 能命中 Join 中的任意分支")

	lab.Section("场景 D：为什么不要用 strings.Contains(err.Error(), \"timeout\")")
	fake := errors.New("config error: timeout is not allowed in this field")
	stringSaysRetry := strings.Contains(strings.ToLower(fake.Error()), "timeout")
	lab.Infof("message=%q stringSaysRetry=%v typeSays=%s", fake, stringSaysRetry, classify(fake))
	lab.Check(stringSaysRetry && classify(fake) == "unknown", "字符串匹配出现误判，类型判断不会")

	lab.Section("场景 E：可重试错误驱动重试，而不是重试所有错误")
	attempts := 0
	var last error
	for attempts < 3 {
		attempts++
		if attempts < 3 {
			last = &RetryableError{Op: "rpc", Err: errors.New("temporary failure")}
			lab.Infof("attempt %d -> retryable", attempts)
			continue
		}
		last = nil
	}
	lab.Infof("attempts=%d last=%v", attempts, last)
	lab.Check(attempts == 3 && last == nil, "只在 retryable 时继续，永久错误立即失败")

	lab.Section("Java → Go 对照")
	lab.Infof("new XxxException(cause)        ≈  fmt.Errorf(\"...: %w\", err) 或自定义类型 + Unwrap（只负责构造/包装）")
	lab.Infof("throw / 向上抛出                 ≈  return fmt.Errorf(\"...: %w\", err)")
	lab.Infof("catch (XxxException e)         ≈  errors.As(err, &target)（对返回的 error 做类型匹配）")
	lab.Infof("e instanceof Sentinel          ≈  errors.Is(err, ErrSentinel)")
	lab.Infof("cause chain                    ≈  Unwrap() error / Unwrap() []error")
	lab.Done()
}
