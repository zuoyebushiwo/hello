package main

func Do[T any](n T) T {
	return n
}

func main() {
	Do[struct{}](struct{}{})
	Do[any]("abc")
}

type GenericStruct[T int | string] struct {
	Name string
	Id   T
}

func (g GenericStruct[T]) name[S int | float64](a S) S {
	return a
}

type Int interface {
	~int8 | ~int16 | ~int | ~int32 | ~int64 | ~uint8 | ~uint16 | ~uint | ~uint32 | ~uint64 | TinyInt // 无法通过编译
}

type TinyInt int8

type Queue[T any] []T

func (q *Queue[T]) Push(e T) {
	*q = append(*q, e)
}

func (q *Queue[T]) Pop(e T) (_ T) {
	if q.Size() > 0 {
		res := q.Peek()
		*q = (*q)[1:]
		return res
	}
	return
}

func (q *Queue[T]) Peek() (_ T) {
	if q.Size() > 0 {
		return (*q)[0]
	}
	return
}

func (q *Queue[T]) Size() int {
	return len(*q)
}
