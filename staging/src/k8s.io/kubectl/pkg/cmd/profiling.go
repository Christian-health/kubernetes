/*
Copyright 2017 The Kubernetes Authors.

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

package cmd

import (
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"

	"github.com/spf13/pflag"
)

var (
	profileName   string
	profileOutput string
)

func addProfilingFlags(flags *pflag.FlagSet) {
	flags.StringVar(&profileName, "profile", "none", "Name of profile to capture. One of (none|cpu|heap|goroutine|threadcreate|block|mutex)")
	flags.StringVar(&profileOutput, "profile-output", "profile.pprof", "Name of the file to write the profile to")
}
/*
	这段代码是一个名为`initProfiling`的函数，用于在Kubernetes（k8s）中初始化性能分析。
	函数首先声明了两个变量`f`和`err`，类型分别为`*os.File`和`error`。
	接下来，根据`profileName`的不同值进行不同的处理：
		- 如果`profileName`为"none"，表示不进行任何性能分析，直接返回nil。
		- 如果`profileName`为"cpu"，则创建一个文件`f`用于存储CPU分析数据，并开始CPU分析。如果创建文件或开始分析过程中出现错误，则返回错误。
		- 如果`profileName`为"block"，则设置阻塞事件的采样率为1。
		- 如果`profileName`为"mutex"，则设置互斥锁事件的采样率为1。
		- 如果`profileName`为其他值，表示指定了一个有效的性能分析名称，通过调用`pprof.Lookup(profileName)`来检查该名称是否有效。如果无效，则返回一个错误。
	然后，处理ctrl+c,代码通过将`os.Interrupt`信号通知到管道`c`，在一个单独的goroutine中监听该信号。当收到该信号时，会关闭文件`f`，然后调用`flushProfiling`函数将性能分析数据刷新到磁盘，并以退出代码0退出程序。
	最后，函数返回nil，表示性能分析初始化成功。
	综上所述，这段代码的含义是根据配置的`profileName`值在Kubernetes中初始化性能分析。根据不同的`profileName`值，可能会创建文件、设置采样率，然后监听中断信号以在中断时进行清理和退出。
*/
func initProfiling() error {
	var (
		f   *os.File
		err error
	)
	switch profileName {
	case "none":
		return nil
	case "cpu":
		f, err = os.Create(profileOutput)
		if err != nil {
			return err
		}
		err = pprof.StartCPUProfile(f)
		if err != nil {
			return err
		}
	// Block and mutex profiles need a call to Set{Block,Mutex}ProfileRate to
	// output anything. We choose to sample all events.
	case "block":
		runtime.SetBlockProfileRate(1)
	case "mutex":
		runtime.SetMutexProfileFraction(1)
	default:
		// Check the profile name is valid.
		if profile := pprof.Lookup(profileName); profile == nil {
			return fmt.Errorf("unknown profile '%s'", profileName)
		}
	}

	// If the command is interrupted before the end (ctrl-c), flush the
	// profiling files
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		f.Close()
		flushProfiling()
		os.Exit(0)
	}()

	return nil
}

/*
	这段代码定义了一个名为`flushProfiling`的函数，用于刷新性能分析数据。
	在函数中，根据`profileName`的不同值进行不同的处理：
		- 如果`profileName`为"none"，则直接返回nil，表示不进行任何性能分析。
		- 如果`profileName`为"cpu"，则停止CPU分析。
		- 如果`profileName`为"heap"，则手动进行一次垃圾回收（GC），然后继续执行下面的代码。
		- 如果`profileName`为其他值，首先检查该值是否是有效的性能分析名称，如果不是，则直接返回nil。
		- 接下来，创建一个文件`f`用于存储性能分析数据，并在结束时关闭文件。
  		- 然后，通过调用`profile.WriteTo(f, 0)`将性能分析数据写入文件中。
	最后，函数返回nil，表示性能分析数据刷新成功。
	综上所述，这段代码的含义是根据配置的`profileName`值刷新性能分析数据。根据不同的`profileName`值，可能会停止分析、进行垃圾回收，然后将性能分析数据写入文件中。
*/
func flushProfiling() error {
	switch profileName {
	case "none":
		return nil
	case "cpu":
		pprof.StopCPUProfile()
	case "heap":
		runtime.GC()
		fallthrough
	default:
		profile := pprof.Lookup(profileName)
		if profile == nil {
			return nil
		}
		f, err := os.Create(profileOutput)
		if err != nil {
			return err
		}
		defer f.Close()
		profile.WriteTo(f, 0)
	}

	return nil
}
