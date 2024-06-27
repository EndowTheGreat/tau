package starlark

import "go.starlark.net/starlark"

type Module interface {
	Name() string
}

type VM interface {
	Module(mod Module)
	Modules(mod ...Module)
	File(module string) (Context, error)
}

type Context interface {
	Call(functionName string, args ...starlark.Value) (starlark.Value, error)
	CallWithNative(functionName string, args ...any) (any, error)
}
