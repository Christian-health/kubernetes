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

package create

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/dynamic"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/cmd/util/editor"
	"k8s.io/kubectl/pkg/generate"
	"k8s.io/kubectl/pkg/rawhttp"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/kubectl/pkg/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)
/*
	这段代码定义了一个名为CreateOptions的结构体，用于存储'create'子命令的命令行选项。
	【子命令应该有自己的Option，比如kubectl run 也有自己的option，所以一定有一个RunOptions结构，存储'run'子命令的命令行选项。】
*/
// CreateOptions is the commandline options for 'create' sub command
type CreateOptions struct {
	// PrintFlags: 用于控制打印输出的标志选项。 kubectl create -o|--output=json|yaml  使用
	PrintFlags  *genericclioptions.PrintFlags
	// RecordFlags: 用于控制记录操作的标志选项。 kubectl create --record 使用
	RecordFlags *genericclioptions.RecordFlags
	// DryRunStrategy: 用于指定干运行策略的选项。 kubectl create --dry-run
	DryRunStrategy cmdutil.DryRunStrategy
	// ValidationDirective: 用于指定验证指令的选项。 kubectl create --validate='strict'
	ValidationDirective string
	/*
		kubectl create  --field-manager='kubectl-create' 使用
		在 kubectl 中，`--field-manager` 是一个用于指定字段管理器的选项。字段管理器用于跟踪对资源对象的修改，并记录修改的来源。
		当使用 `kubectl create` 命令创建资源对象时，可以通过 `--field-manager` 选项指定一个字段管理器。字段管理器是一个标识符，用于标记对资源对象的修改操作。它可以帮助跟踪和管理对资源对象所做的更改，并在需要时进行回溯。
		字段管理器的作用包括：
		1. 标识修改来源：通过指定字段管理器，可以在后续的操作中识别和追踪对资源对象的修改。这对于多个用户或多个工具同时对同一资源对象进行操作时很有用，可以清楚地了解每个修改的来源。
		2. 避免冲突：字段管理器可以协调多个修改操作，以避免冲突。当多个用户或多个工具同时对同一资源对象进行修改时，字段管理器可以确保修改操作按照正确的顺序进行，从而避免冲突和数据不一致的问题。
		3. 回溯修改历史：通过字段管理器，可以追溯和记录对资源对象的修改历史。这对于审计和故障排查很有帮助，可以了解每个修改操作的详细信息，包括修改的时间、修改的用户或工具等。
		总之，`--field-manager` 选项用于指定字段管理器，它在创建资源对象时起到标识和管理修改操作的作用。通过使用字段管理器，可以更好地追踪、协调和记录对资源对象的修改，提高操作的可追溯性和可管理性。
	 */
	fieldManager string
	// FilenameOptions: 用于指定文件名选项的选项。 kubectl create  -f, --filename=[]
	FilenameOptions  resource.FilenameOptions
	// Selector: 用于指定选择器的选项。 kubectl create -l, --selector=''
	Selector         string
	// EditBeforeCreate: 用于指定在创建之前是否编辑的选项。 kubectl create --edit=false
	EditBeforeCreate bool
	// Raw: 用于指定原始输入的选项。 kubectl create --raw=''
	Raw              string
	//  Recorder: 用于记录操作的选项。比如添加一个注解annotations
	Recorder genericclioptions.Recorder
	//  PrintObj: 用于打印对象的函数。
	PrintObj func(obj kruntime.Object) error
	// IOStreams: 用于输入输出流的选项。
	genericiooptions.IOStreams
}

var (
	createLong = templates.LongDesc(i18n.T(`
		Create a resource from a file or from stdin.

		JSON and YAML formats are accepted.`))

	createExample = templates.Examples(i18n.T(`
		# Create a pod using the data in pod.json
		kubectl create -f ./pod.json

		# Create a pod based on the JSON passed into stdin
		cat pod.json | kubectl create -f -

		# Edit the data in registry.yaml in JSON then create the resource using the edited data
		kubectl create -f registry.yaml --edit -o json`))
)

// NewCreateOptions returns an initialized CreateOptions instance
func NewCreateOptions(ioStreams genericiooptions.IOStreams) *CreateOptions {
	return &CreateOptions{
		PrintFlags:  genericclioptions.NewPrintFlags("created").WithTypeSetter(scheme.Scheme),
		RecordFlags: genericclioptions.NewRecordFlags(),
		// 默认的Recorder，后面的代码会重新的进行设置
		Recorder: genericclioptions.NoopRecorder{},

		IOStreams: ioStreams,
	}
}

// NewCmdCreate returns new initialized instance of create sub command
func NewCmdCreate(f cmdutil.Factory, ioStreams genericiooptions.IOStreams) *cobra.Command {
	o := NewCreateOptions(ioStreams)

	cmd := &cobra.Command{
		Use:                   "create -f FILENAME",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Create a resource from a file or from stdin"),
		Long:                  createLong,
		Example:               createExample,
		Run: func(cmd *cobra.Command, args []string) {
			// 首先检查是否指定了文件名或 Kustomize 配置，如果没有指定则打印错误信息并返回
			if cmdutil.IsFilenameSliceEmpty(o.FilenameOptions.Filenames, o.FilenameOptions.Kustomize) {
				ioStreams.ErrOut.Write([]byte("Error: must specify one of -f and -k\n\n"))
				defaultRunFunc := cmdutil.DefaultSubCommandRun(ioStreams.ErrOut)
				defaultRunFunc(cmd, args)
				return
			}
			// 调用 `o.Complete()` 完成命令的初始化
			cmdutil.CheckErr(o.Complete(f, cmd, args))
			// 调用 `o.Validate()` 验证命令的参数和选项
			cmdutil.CheckErr(o.Validate())
			// 最后调用 `o.RunCreate()` 执行创建资源的逻辑。
			cmdutil.CheckErr(o.RunCreate(f, cmd))
		},
	}

	// bind flag structs
	// 处理kubectl create --record 这个选项，获取命令--record选项值
	o.RecordFlags.AddFlags(cmd)

	usage := "to use to create the resource"
	// 处理kubectl create --filename kustomize recursive 这些选项，获取命令的这些选项参数值
	cmdutil.AddFilenameOptionFlags(cmd, &o.FilenameOptions, usage)
	// 处理kubectl create  --validate 这个选项，获取命令的这个选项参数值
	cmdutil.AddValidateFlags(cmd)
	// 处理kubectl create  --edit 这个选项，获取命令的这个选项参数值
	cmd.Flags().BoolVar(&o.EditBeforeCreate, "edit", o.EditBeforeCreate, "Edit the API resource before creating")
	// 处理kubectl create  --windows-line-endings 这个选项，获取命令的这个选项参数值
	cmd.Flags().Bool("windows-line-endings", runtime.GOOS == "windows",
		"Only relevant if --edit=true. Defaults to the line ending native to your platform.")
	cmdutil.AddApplyAnnotationFlags(cmd)
	// 处理kubectl create --dry-run 这个选项，获取命令的这个选项参数值
	cmdutil.AddDryRunFlag(cmd)
	// 处理kubectl create --selector 这个选项，获取命令的这个选项参数值
	cmdutil.AddLabelSelectorFlagVar(cmd, &o.Selector)
	// 处理kubectl create --raw 这个选项，获取命令的这个选项参数值
	cmd.Flags().StringVar(&o.Raw, "raw", o.Raw, "Raw URI to POST to the server.  Uses the transport specified by the kubeconfig file.")
	// 处理kubectl create --field-manager 这个选项，获取命令的这个选项参数值
	cmdutil.AddFieldManagerFlagVar(cmd, &o.fieldManager, "kubectl-create")

	// 处理kubectl create --output 这个选项，获取命令的这个选项参数值
	o.PrintFlags.AddFlags(cmd)

	// create subcommands
	// kubectl create namespace 子命令
	cmd.AddCommand(NewCmdCreateNamespace(f, ioStreams))
	// kubectl create quota 子命令
	cmd.AddCommand(NewCmdCreateQuota(f, ioStreams))
	// kubectl create secret 子命令
	cmd.AddCommand(NewCmdCreateSecret(f, ioStreams))
	// kubectl create configmap 子命令
	cmd.AddCommand(NewCmdCreateConfigMap(f, ioStreams))
	// kubectl create serviceaccout 子命令
	cmd.AddCommand(NewCmdCreateServiceAccount(f, ioStreams))
	// kubectl create service 子命令
	cmd.AddCommand(NewCmdCreateService(f, ioStreams))
	// kubectl create deployment 子命令
	cmd.AddCommand(NewCmdCreateDeployment(f, ioStreams))
	// kubectl create clusterrole 子命令
	cmd.AddCommand(NewCmdCreateClusterRole(f, ioStreams))
	// kubectl create clusterrolebinding 子命令
	cmd.AddCommand(NewCmdCreateClusterRoleBinding(f, ioStreams))
	// kubectl create role 子命令
	cmd.AddCommand(NewCmdCreateRole(f, ioStreams))
	// kubectl create rolebinding 子命令
	cmd.AddCommand(NewCmdCreateRoleBinding(f, ioStreams))
	// kubectl create poddisruptionbudget 子命令
	cmd.AddCommand(NewCmdCreatePodDisruptionBudget(f, ioStreams))
	// kubectl create priorityclass 子命令
	cmd.AddCommand(NewCmdCreatePriorityClass(f, ioStreams))
	// kubectl create job 子命令
	cmd.AddCommand(NewCmdCreateJob(f, ioStreams))
	// kubectl create cronjob 子命令
	cmd.AddCommand(NewCmdCreateCronJob(f, ioStreams))
	// kubectl create ingress 子命令
	cmd.AddCommand(NewCmdCreateIngress(f, ioStreams))
	// kubectl create token 子命令
	cmd.AddCommand(NewCmdCreateToken(f, ioStreams))
	return cmd
}

/*
	如下代码定义了一个CreateOptions结构体的Validate方法，用于验证命令选项的一致性。
*/
// Validate makes sure there is no discrepency in command options
func (o *CreateOptions) Validate() error {
	// 在Validate方法中，首先判断是否使用了"--raw"选项。如果使用了"--raw"选项，则进行一系列的验证。
	if len(o.Raw) > 0 {
		// 如果同时使用了"--raw"和"--edit"选项，则返回错误，因为这两个选项是互斥的。
		if o.EditBeforeCreate {
			return fmt.Errorf("--raw and --edit are mutually exclusive")
		}
		// 如果"--raw"选项使用了多个本地文件或标准输入作为输入，则返回错误，因为"--raw"选项只能使用单个本地文件或标准输入。
		if len(o.FilenameOptions.Filenames) != 1 {
			return fmt.Errorf("--raw can only use a single local file or stdin")
		}
		// 	如果"--raw"选项使用了URL作为输入，则返回错误，因为"--raw"选项不能从URL读取。
		if strings.Index(o.FilenameOptions.Filenames[0], "http://") == 0 || strings.Index(o.FilenameOptions.Filenames[0], "https://") == 0 {
			return fmt.Errorf("--raw cannot read from a url")
		}
		// 如果同时使用了"--raw"和"--recursive"选项，则返回错误，因为这两个选项是互斥的。
		if o.FilenameOptions.Recursive {
			return fmt.Errorf("--raw and --recursive are mutually exclusive")
		}
		// 如果同时使用了"--raw"和"--selector"（或"-l"的别名）选项，则返回错误，因为这两个选项是互斥的。
		if len(o.Selector) > 0 {
			return fmt.Errorf("--raw and --selector (-l) are mutually exclusive")
		}
		// 	如果同时使用了"--raw"和"--output"选项，则返回错误，因为这两个选项是互斥的。
		if o.PrintFlags.OutputFormat != nil && len(*o.PrintFlags.OutputFormat) > 0 {
			return fmt.Errorf("--raw and --output are mutually exclusive")
		}
		// 最后，如果"--raw"选项的值无法解析为有效的URL路径，则返回错误。
		if _, err := url.ParseRequestURI(o.Raw); err != nil {
			return fmt.Errorf("--raw must be a valid URL path: %v", err)
		}
	}
	// 如果没有使用"--raw"选项，则直接返回nil，表示验证通过
	return nil
}
/*
	如下代码定义了一个CreateOptions结构体的Complete方法，用于补全所有必需的选项。
*/
// Complete completes all the required options
func (o *CreateOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return cmdutil.UsageErrorf(cmd, "Unexpected args: %v", args)
	}
	var err error
	// 接下来，Complete方法补全了RecordFlags选项。它调用RecordFlags.Complete方法来处理与记录相关的选项，并将结果赋值给CreateOptions结构体的Recorder字段。
	o.RecordFlags.Complete(cmd)
	o.Recorder, err = o.RecordFlags.ToRecorder()
	if err != nil {
		return err
	}
	// 然后，补全了DryRunStrategy，Complete方法获取DryRunStrategy选项的值，并将其赋值给CreateOptions结构体的DryRunStrategy字段。
	o.DryRunStrategy, err = cmdutil.GetDryRunStrategy(cmd)
	if err != nil {
		return err
	}
	// 调用cmdutil.PrintFlagsWithDryRunStrategy方法，将PrintFlags选项与DryRunStrategy选项结合起来打印。
	cmdutil.PrintFlagsWithDryRunStrategy(o.PrintFlags, o.DryRunStrategy)
	// 	Complete方法继续获取ValidationDirective选项的值，并将其赋值给CreateOptions结构体的ValidationDirective字段。补全了ValidationDirective
	o.ValidationDirective, err = cmdutil.GetValidationDirective(cmd)
	if err != nil {
		return err
	}
	// 接下来，Complete方法将PrintFlags选项转换为打印器（printer）对象，并将其赋值给CreateOptions结构体的PrintObj字段。
	printer, err := o.PrintFlags.ToPrinter()
	if err != nil {
		return err
	}

	o.PrintObj = func(obj kruntime.Object) error {
		return printer.PrintObj(obj, o.Out)
	}
	// 	最后，Complete方法返回nil，表示所有选项都已成功完成。
	return nil
}
/*
	如下函数用于执行创建资源对象的操作。
	该函数主要负责解析命令行参数、构建资源对象、执行创建操作，并处理相关的错误和结果。
*/
// RunCreate performs the creation
func (o *CreateOptions) RunCreate(f cmdutil.Factory, cmd *cobra.Command) error {
	// raw only makes sense for a single file resource multiple objects aren't likely to do what you want.
	// the validator enforces this, so
	// 首先判断是否存在 `o.Raw`，如果存在，则使用 `rawhttp.RawPost` 函数将原始数据直接发送到 Kubernetes API Server 进行创建。
	if len(o.Raw) > 0 {
		restClient, err := f.RESTClient()
		if err != nil {
			return err
		}
		return rawhttp.RawPost(restClient, o.IOStreams, o.Raw, o.FilenameOptions.Filenames[0])
	}
	// 如果 `o.EditBeforeCreate` 为 true，则调用 `RunEditOnCreate` 函数，在创建之前对资源对象进行编辑。
	if o.EditBeforeCreate {
		return RunEditOnCreate(f, o.PrintFlags, o.RecordFlags, o.IOStreams, cmd, &o.FilenameOptions, o.fieldManager)
	}
	// 获取资源对象的验证器 `schema`。
	schema, err := f.Validator(o.ValidationDirective)
	if err != nil {
		return err
	}
	// 获取当前命令的命名空间和是否强制使用命名空间的标志。
	cmdNamespace, enforceNamespace, err := f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}
	// 使用资源构建器 `f.NewBuilder()` 创建一个资源构建器，并设置相关参数，如命名空间、文件名、标签选择器等。
	// Builder的结构体，用于提供方便的函数来从命令行参数中获取参数，并将它们转换为一个资源列表，以便使用Visitor接口进行迭代。
	r := f.NewBuilder().
		// 用于更新构建器，以便请求和发送非结构化对象（unstructured objects）。
		Unstructured().
		Schema(schema).
		ContinueOnError().
		NamespaceParam(cmdNamespace).DefaultNamespace().
		FilenameParam(enforceNamespace, &o.FilenameOptions).
		LabelSelectorParam(o.Selector).
		Flatten().
		// 调用 `Do()` 函数执行创建操作，并返回一个资源信息对象 `r`。
		Do()
	err = r.Err()
	if err != nil {
		return err
	}

	count := 0
	err = r.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		// 调用util.CreateOrUpdateAnnotation函数为资源对象添加或更新注解
		if err := util.CreateOrUpdateAnnotation(cmdutil.GetFlagBool(cmd, cmdutil.ApplyAnnotationsFlag), info.Object, scheme.DefaultJSONEncoder()); err != nil {
			return cmdutil.AddSourceToErr("creating", info.Source, err)
		}
		// 记录资源对象的操作
		if err := o.Recorder.Record(info.Object); err != nil {
			klog.V(4).Infof("error recording current command: %v", err)
		}

		if o.DryRunStrategy != cmdutil.DryRunClient {
			// 根据指定的验证策略和字段管理器，使用资源助手 `resource.Helper` 执行创建操作。
			obj, err := resource.
				NewHelper(info.Client, info.Mapping).
				DryRun(o.DryRunStrategy == cmdutil.DryRunServer).
				WithFieldManager(o.fieldManager).
				WithFieldValidation(o.ValidationDirective).
				Create(info.Namespace, true, info.Object)
			if err != nil {
				return cmdutil.AddSourceToErr("creating", info.Source, err)
			}
			// 更新资源信息对象的状态
			info.Refresh(obj, true)
		}

		count++
		// 打印资源对象
		return o.PrintObj(info.Object)
	})
	if err != nil {
		return err
	}
	// 如果创建的资源对象数量为 0，则返回错误。
	if count == 0 {
		return fmt.Errorf("no objects passed to create")
	}
	// 返回 nil，表示创建操作成功。
	return nil
}
/*
	这段代码定义了一个名为RunEditOnCreate的函数，用于在创建资源时执行编辑操作。
	在函数中，首先创建一个EditOptions对象editOptions，并将其设置为编辑模式为EditBeforeCreateMode，同时传入ioStreams参数。
	接下来，将传入的options参数赋值给editOptions的FilenameOptions字段。
	然后，获取cmd命令中的ValidationDirective选项的值，并将其转换为字符串类型，并设置给editOptions的ValidateOptions字段的ValidationDirective字段。
	接着，将传入的printFlags参数赋值给editOptions的PrintFlags字段。
	通过cmdutil.GetFlagBool方法获取cmd命令中的ApplyAnnotationsFlag选项的布尔值，并将其赋值给editOptions的ApplyAnnotation字段。
	将传入的recordFlags参数赋值给editOptions的RecordFlags字段。
	将字符串"kubectl-create"赋值给editOptions的FieldManager字段。
	接下来，通过调用editOptions的Complete方法，完成所有必需的选项。
	最后，调用editOptions的Run方法，执行编辑操作。
	如果任何步骤中发生错误，错误将被返回，否则返回nil。
*/
// RunEditOnCreate performs edit on creation
func RunEditOnCreate(f cmdutil.Factory, printFlags *genericclioptions.PrintFlags, recordFlags *genericclioptions.RecordFlags, ioStreams genericiooptions.IOStreams, cmd *cobra.Command, options *resource.FilenameOptions, fieldManager string) error {
	editOptions := editor.NewEditOptions(editor.EditBeforeCreateMode, ioStreams)
	editOptions.FilenameOptions = *options
	validationDirective, err := cmdutil.GetValidationDirective(cmd)
	if err != nil {
		return err
	}
	editOptions.ValidateOptions = cmdutil.ValidateOptions{
		ValidationDirective: string(validationDirective),
	}
	editOptions.PrintFlags = printFlags
	editOptions.ApplyAnnotation = cmdutil.GetFlagBool(cmd, cmdutil.ApplyAnnotationsFlag)
	editOptions.RecordFlags = recordFlags
	editOptions.FieldManager = "kubectl-create"

	err = editOptions.Complete(f, []string{}, cmd)
	if err != nil {
		return err
	}
	return editOptions.Run()
}

// NameFromCommandArgs is a utility function for commands that assume the first argument is a resource name
func NameFromCommandArgs(cmd *cobra.Command, args []string) (string, error) {
	argsLen := cmd.ArgsLenAtDash()
	// ArgsLenAtDash returns -1 when -- was not specified
	if argsLen == -1 {
		argsLen = len(args)
	}
	if argsLen != 1 {
		return "", cmdutil.UsageErrorf(cmd, "exactly one NAME is required, got %d", argsLen)
	}
	return args[0], nil
}
/*
	如下代码定义了一个名为CreateSubcommandOptions的结构体，用于支持创建子命令。
	CreateSubcommandOptions结构体包含了以下字段：
	- PrintFlags：一个指向genericclioptions.PrintFlags的指针，用于获取打印器的选项。
	- Name：被创建资源的名称。
	- StructuredGenerator：用于创建对象的资源生成器。
	- DryRunStrategy：干运行策略，用于模拟创建操作而不实际执行。
	- CreateAnnotation：一个布尔值，表示是否在创建资源时添加注释。
	- FieldManager：用于标识创建资源的字段管理器。
	- ValidationDirective：验证指令，用于指定创建操作的验证方式。

	- Namespace：指定资源所属的命名空间。
	- EnforceNamespace：一个布尔值，表示是否强制执行命名空间。
	- Mapper：用于将资源API对象映射到REST接口的RESTMapper。
	- DynamicClient：动态客户端，用于与Kubernetes API进行交互。

	- PrintObj：资源打印函数，用于将资源打印到输出流中。

	- IOStreams：用于输入和输出流的IOStreams对象，包括输入、输出和错误输出。

	CreateSubcommandOptions结构体提供了创建子命令所需的各种选项和参数。
*/
// CreateSubcommandOptions is an options struct to support create subcommands
type CreateSubcommandOptions struct {
	// PrintFlags holds options necessary for obtaining a printer
	PrintFlags *genericclioptions.PrintFlags
	// Name of resource being created
	Name string
	// StructuredGenerator is the resource generator for the object being created
	StructuredGenerator generate.StructuredGenerator
	DryRunStrategy      cmdutil.DryRunStrategy
	CreateAnnotation    bool
	FieldManager        string
	ValidationDirective string

	Namespace        string
	EnforceNamespace bool

	Mapper        meta.RESTMapper
	DynamicClient dynamic.Interface

	PrintObj printers.ResourcePrinterFunc

	genericiooptions.IOStreams
}

// NewCreateSubcommandOptions returns initialized CreateSubcommandOptions
func NewCreateSubcommandOptions(ioStreams genericiooptions.IOStreams) *CreateSubcommandOptions {
	return &CreateSubcommandOptions{
		PrintFlags: genericclioptions.NewPrintFlags("created").WithTypeSetter(scheme.Scheme),
		IOStreams:  ioStreams,
	}
}
/*
	下面代码是一个在 Kubernetes (k8s) 中用于完成创建子命令选项的方法，它属于 `CreateSubcommandOptions` 结构体的方法。
	该方法通过获取和设置各种选项的值，准备了创建子命令所需的所有信息，以便在后续的操作中使用。
*/
// Complete completes all the required options
func (o *CreateSubcommandOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string, generator generate.StructuredGenerator) error {
	// 从命令行参数中获取名称，并将其赋值给 `CreateSubcommandOptions` 结构体的 `Name` 字段。
	name, err := NameFromCommandArgs(cmd, args)
	if err != nil {
		return err
	}

	o.Name = name
	// 将传入的 `generator` 参数赋值给 `CreateSubcommandOptions` 结构体的 `StructuredGenerator` 字段。
	o.StructuredGenerator = generator
	// 通过调用 `cmdutil.GetDryRunStrategy()` 方法获取干预策略，并将其赋值给 `CreateSubcommandOptions` 结构体的 `DryRunStrategy` 字段。
	o.DryRunStrategy, err = cmdutil.GetDryRunStrategy(cmd)
	if err != nil {
		return err
	}
	// 通过调用 `cmdutil.GetFlagBool()` 方法获取标志位的布尔值，并将其赋值给 `CreateSubcommandOptions` 结构体的 `CreateAnnotation` 字段。
	o.CreateAnnotation = cmdutil.GetFlagBool(cmd, cmdutil.ApplyAnnotationsFlag)
	// 调用 `cmdutil.PrintFlagsWithDryRunStrategy()` 方法打印标志位及干预策略。
	cmdutil.PrintFlagsWithDryRunStrategy(o.PrintFlags, o.DryRunStrategy)
	// 通过调用 `o.PrintFlags.ToPrinter()` 方法获取打印机对象，并将其赋值给 `CreateSubcommandOptions` 结构体的 `PrintObj` 字段。
	printer, err := o.PrintFlags.ToPrinter()
	if err != nil {
		return err
	}
	// 通过调用 `cmdutil.GetValidationDirective()` 方法获取验证指令，并将其赋值给 `CreateSubcommandOptions` 结构体的 `ValidationDirective` 字段。
	o.ValidationDirective, err = cmdutil.GetValidationDirective(cmd)
	if err != nil {
		return err
	}

	o.PrintObj = func(obj kruntime.Object, out io.Writer) error {
		return printer.PrintObj(obj, out)
	}
	// 通过调用 `f.ToRawKubeConfigLoader().Namespace()` 方法获取命名空间信息，并将其赋值给 `CreateSubcommandOptions` 结构体的 `Namespace` 和 `EnforceNamespace` 字段。
	o.Namespace, o.EnforceNamespace, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}
	// 通过调用 `f.DynamicClient()` 方法获取动态客户端对象，并将其赋值给 `CreateSubcommandOptions` 结构体的 `DynamicClient` 字段。
	o.DynamicClient, err = f.DynamicClient()
	if err != nil {
		return err
	}
	// 通过调用 `f.ToRESTMapper()` 方法获取 REST 映射器对象，并将其赋值给 `CreateSubcommandOptions` 结构体的 `Mapper` 字段。
	o.Mapper, err = f.ToRESTMapper()
	if err != nil {
		return err
	}
	// 返回 `nil` 以表示方法执行成功。
	return nil
}
/*
	如下代码是一个在 Kubernetes (k8s) 中用于执行创建子命令的方法，它属于 `CreateSubcommandOptions` 结构体的方法。
	该方法负责执行创建子命令的实际操作，包括生成 API 对象、创建或更新注释、执行创建操作，并将结果打印到输出流中。
*/
// Run executes a create subcommand using the specified options
func (o *CreateSubcommandOptions) Run() error {
	// 调用 `o.StructuredGenerator.StructuredGenerate()` 方法生成一个 API 对象。
	obj, err := o.StructuredGenerator.StructuredGenerate()
	if err != nil {
		return err
	}
	//  调用 `util.CreateOrUpdateAnnotation()` 方法创建或更新注释，以便记录创建操作的相关信息。
	if err := util.CreateOrUpdateAnnotation(o.CreateAnnotation, obj, scheme.DefaultJSONEncoder()); err != nil {
		return err
	}
	// 如果创建操作不是在客户端模拟执行，则进行以下步骤：
	if o.DryRunStrategy != cmdutil.DryRunClient {
		// create subcommands have compiled knowledge of things they create, so type them directly
		gvks, _, err := scheme.Scheme.ObjectKinds(obj)
		if err != nil {
			return err
		}
		gvk := gvks[0]
		mapping, err := o.Mapper.RESTMapping(schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}, gvk.Version)
		if err != nil {
			return err
		}
		// 将生成的 API 对象转换为 `unstructured.Unstructured` 类型的对象。
		asUnstructured := &unstructured.Unstructured{}
		if err := scheme.Scheme.Convert(obj, asUnstructured, nil); err != nil {
			return err
		}
		// 如果映射器的作用域为根级别，则将命名空间设置为空字符串。
		if mapping.Scope.Name() == meta.RESTScopeNameRoot {
			o.Namespace = ""
		}
		// 创建一个 `metav1.CreateOptions` 对象，并根据需要设置字段管理器和字段验证指令。
		createOptions := metav1.CreateOptions{}
		if o.FieldManager != "" {
			createOptions.FieldManager = o.FieldManager
		}
		createOptions.FieldValidation = o.ValidationDirective
		//  如果干预策略为在服务器端模拟执行，则将干预策略设置为 `metav1.DryRunAll`。
		if o.DryRunStrategy == cmdutil.DryRunServer {
			createOptions.DryRun = []string{metav1.DryRunAll}
		}
		// 调用 `o.DynamicClient.Resource(mapping.Resource).Namespace(o.Namespace).Create()` 方法创建实际的对象，并将其返回给 `actualObject`。
		actualObject, err := o.DynamicClient.Resource(mapping.Resource).Namespace(o.Namespace).Create(context.TODO(), asUnstructured, createOptions)
		if err != nil {
			return err
		}

		// ensure we pass a versioned object to the printer
		// 确保将版本化的对象传递给打印机。
		obj = actualObject
	} else {// 否则，如果生成的对象实现了 `meta.Accessor` 接口，并且 `o.EnforceNamespace` 为 true，则将对象的命名空间设置为指定的命名空间。
		if meta, err := meta.Accessor(obj); err == nil && o.EnforceNamespace {
			meta.SetNamespace(o.Namespace)
		}
	}
	// 返回调用 `o.PrintObj()` 方法打印生成的对象到输出流的结果。
	return o.PrintObj(obj, o.Out)
}
