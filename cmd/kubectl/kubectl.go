/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"k8s.io/component-base/cli"
	"k8s.io/kubectl/pkg/cmd"
	"k8s.io/kubectl/pkg/cmd/util"

	// Import to initialize client auth plugins.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func main() {
	// 调用"cmd.NewDefaultKubectlCommand()"创建了一个默认的Kubectl命令对象。这个对象包含了Kubectl命令行工具的所有命令和子命令。
	command := cmd.NewDefaultKubectlCommand()
	// 调用"cli.RunNoErrOutput(command)"函数来执行Kubectl命令，并且不输出错误信息。这个函数会执行命令并检查错误.
	if err := cli.RunNoErrOutput(command); err != nil {
		// Pretty-print the error and exit with an error.
		// 如果有错误发生，则会使用"util.CheckErr(err)"函数将错误信息以漂亮的格式打印出来，并以错误状态退出程序。
		util.CheckErr(err)
	}
}
