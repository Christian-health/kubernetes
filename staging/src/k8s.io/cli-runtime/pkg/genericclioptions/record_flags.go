/*
Copyright 2018 The Kubernetes Authors.

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

package genericclioptions

import (
	"os"
	"path/filepath"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
)

// ChangeCauseAnnotation is the annotation indicating a guess at "why" something was changed
const ChangeCauseAnnotation = "kubernetes.io/change-cause"
/*
	如下代码定义了一个名为RecordFlags的结构体，它包含与"--record"操作相关的所有标志。
	RecordFlags结构体包含以下字段：
	- Record：一个指向bool类型的指针，用于指示记录标志的状态。通过修改该指针的值，调用者可以选择是否启用记录功能或重新绑定。
	- changeCause：一个字符串，用于记录状态变更的原因。
	在Kubernetes中，"--record"标志用于启用资源操作的记录功能。当启用记录功能时，Kubernetes会将资源操作的详细信息保存到资源的事件日志中，
	包括操作类型、操作者、时间戳等。通过使用RecordFlags结构体，可以方便地管理与记录功能相关的标志，并记录状态变更的原因。
*/
// RecordFlags contains all flags associated with the "--record" operation
type RecordFlags struct {
	// Record indicates the state of the recording flag.  It is a pointer so a caller can opt out or rebind
	Record *bool

	changeCause string
}

// ToRecorder returns a ChangeCause recorder if --record=false was not
// explicitly given by the user
func (f *RecordFlags) ToRecorder() (Recorder, error) {
	if f == nil {
		return NoopRecorder{}, nil
	}

	shouldRecord := false
	if f.Record != nil {
		shouldRecord = *f.Record
	}

	// if flag was explicitly set to false by the user,
	// do not record
	if !shouldRecord {
		return NoopRecorder{}, nil
	}

	return &ChangeCauseRecorder{
		changeCause: f.changeCause,
	}, nil
}

// Complete is called before the command is run, but after it is invoked to finish the state of the struct before use.
func (f *RecordFlags) Complete(cmd *cobra.Command) error {
	if f == nil {
		return nil
	}

	f.changeCause = parseCommandArguments(cmd)
	return nil
}

// CompleteWithChangeCause alters changeCause value with a new cause
func (f *RecordFlags) CompleteWithChangeCause(cause string) error {
	if f == nil {
		return nil
	}

	f.changeCause = cause
	return nil
}

// AddFlags binds the requested flags to the provided flagset
// TODO have this only take a flagset
func (f *RecordFlags) AddFlags(cmd *cobra.Command) {
	if f == nil {
		return
	}

	if f.Record != nil {
		cmd.Flags().BoolVar(f.Record, "record", *f.Record, "Record current kubectl command in the resource annotation. If set to false, do not record the command. If set to true, record the command. If not set, default to updating the existing annotation value only if one already exists.")
		cmd.Flags().MarkDeprecated("record", "--record will be removed in the future")
	}
}

// NewRecordFlags provides a RecordFlags with reasonable default values set for use
func NewRecordFlags() *RecordFlags {
	record := false

	return &RecordFlags{
		Record: &record,
	}
}

// Recorder is used to record why a runtime.Object was changed in an annotation.
type Recorder interface {
	// Record records why a runtime.Object was changed in an annotation.
	Record(runtime.Object) error
	MakeRecordMergePatch(runtime.Object) ([]byte, error)
}
/*
	上述代码定义了一个名为 `NoopRecorder` 的结构体，它是一个空记录器（Noop Recorder）。
	在 Kubernetes 中，记录器（Recorder）用于记录集群中发生的事件和状态变化。它可以用于跟踪和记录重要的操作、故障和警报等信息，以便后续分析和故障排查。
	`NoopRecorder` 结构体是一个空实现的记录器，它不执行任何操作。它被定义为一个空结构体，没有任何字段或方法。它的主要作用是作为一个“什么都不做”的占位符，可以在代码中返回它，以避免对空记录器进行切换或处理。
	在某些情况下，当不需要记录事件或状态变化时，可以使用 `NoopRecorder`。例如，在测试代码或实现中，如果不关心记录器的具体实现，或者只需要一个空记录器来满足接口或函数的要求，就可以使用它。
	总之，`NoopRecorder` 是一个空记录器的定义，它不执行任何操作，常用于占位或避免对空记录器进行处理的情况。
*/
// NoopRecorder does nothing.  It is a "do nothing" that can be returned so code doesn't switch on it.
type NoopRecorder struct{}

// Record implements Recorder
func (r NoopRecorder) Record(obj runtime.Object) error {
	return nil
}

// MakeRecordMergePatch implements Recorder
func (r NoopRecorder) MakeRecordMergePatch(obj runtime.Object) ([]byte, error) {
	return nil, nil
}

// ChangeCauseRecorder annotates a "change-cause" to an input runtime object
type ChangeCauseRecorder struct {
	changeCause string
}

// Record annotates a "change-cause" to a given info if either "shouldRecord" is true,
// or the resource info previously contained a "change-cause" annotation.
func (r *ChangeCauseRecorder) Record(obj runtime.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	annotations := accessor.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[ChangeCauseAnnotation] = r.changeCause
	accessor.SetAnnotations(annotations)
	return nil
}

// MakeRecordMergePatch produces a merge patch for updating the recording annotation.
func (r *ChangeCauseRecorder) MakeRecordMergePatch(obj runtime.Object) ([]byte, error) {
	// copy so we don't mess with the original
	objCopy := obj.DeepCopyObject()
	if err := r.Record(objCopy); err != nil {
		return nil, err
	}

	oldData, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	newData, err := json.Marshal(objCopy)
	if err != nil {
		return nil, err
	}

	return jsonpatch.CreateMergePatch(oldData, newData)
}

// parseCommandArguments will stringify and return all environment arguments ie. a command run by a client
// using the factory.
// Set showSecrets false to filter out stuff like secrets.
func parseCommandArguments(cmd *cobra.Command) string {
	if len(os.Args) == 0 {
		return ""
	}

	flags := ""
	parseFunc := func(flag *pflag.Flag, value string) error {
		flags = flags + " --" + flag.Name
		if set, ok := flag.Annotations["classified"]; !ok || len(set) == 0 {
			flags = flags + "=" + value
		} else {
			flags = flags + "=CLASSIFIED"
		}
		return nil
	}
	var err error
	err = cmd.Flags().ParseAll(os.Args[1:], parseFunc)
	if err != nil || !cmd.Flags().Parsed() {
		return ""
	}

	args := ""
	if arguments := cmd.Flags().Args(); len(arguments) > 0 {
		args = " " + strings.Join(arguments, " ")
	}

	base := filepath.Base(os.Args[0])
	return base + args + flags
}
