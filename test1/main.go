package main

import "fmt"

var x int // 声明
var y int

func main() {

	x = 10 // 赋值

	y := 20 // 声明并赋值，类型自动推断为 int
	x = 30
	fmt.Print(x, y)
}
