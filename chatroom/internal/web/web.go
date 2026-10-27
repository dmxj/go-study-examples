// Package web 通过 embed 把前端静态文件编译进二进制。
package web

import (
	"embed"
	"io/fs"
)

// files 内嵌 static 目录。go:embed 指令必须紧挨着变量声明，路径相对于本源文件所在目录，
// 且不能包含 ".."——这也是为什么 web 包要和 static 目录放在一起。
//
//go:embed static
var files embed.FS

// Static 返回以 static/ 为根的文件系统，使 http.FileServerFS 能直接用 "/" 访问 index.html。
func Static() fs.FS {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err) // 只可能是编译期就确定的路径错误
	}
	return sub
}
