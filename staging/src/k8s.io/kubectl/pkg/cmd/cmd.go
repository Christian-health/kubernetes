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

package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/cmd/annotate"
	"k8s.io/kubectl/pkg/cmd/apiresources"
	"k8s.io/kubectl/pkg/cmd/apply"
	"k8s.io/kubectl/pkg/cmd/attach"
	"k8s.io/kubectl/pkg/cmd/auth"
	"k8s.io/kubectl/pkg/cmd/autoscale"
	"k8s.io/kubectl/pkg/cmd/certificates"
	"k8s.io/kubectl/pkg/cmd/clusterinfo"
	"k8s.io/kubectl/pkg/cmd/completion"
	cmdconfig "k8s.io/kubectl/pkg/cmd/config"
	"k8s.io/kubectl/pkg/cmd/cp"
	"k8s.io/kubectl/pkg/cmd/create"
	"k8s.io/kubectl/pkg/cmd/debug"
	"k8s.io/kubectl/pkg/cmd/delete"
	"k8s.io/kubectl/pkg/cmd/describe"
	"k8s.io/kubectl/pkg/cmd/diff"
	"k8s.io/kubectl/pkg/cmd/drain"
	"k8s.io/kubectl/pkg/cmd/edit"
	"k8s.io/kubectl/pkg/cmd/events"
	cmdexec "k8s.io/kubectl/pkg/cmd/exec"
	"k8s.io/kubectl/pkg/cmd/explain"
	"k8s.io/kubectl/pkg/cmd/expose"
	"k8s.io/kubectl/pkg/cmd/get"
	"k8s.io/kubectl/pkg/cmd/label"
	"k8s.io/kubectl/pkg/cmd/logs"
	"k8s.io/kubectl/pkg/cmd/options"
	"k8s.io/kubectl/pkg/cmd/patch"
	"k8s.io/kubectl/pkg/cmd/plugin"
	"k8s.io/kubectl/pkg/cmd/portforward"
	"k8s.io/kubectl/pkg/cmd/proxy"
	"k8s.io/kubectl/pkg/cmd/replace"
	"k8s.io/kubectl/pkg/cmd/rollout"
	"k8s.io/kubectl/pkg/cmd/run"
	"k8s.io/kubectl/pkg/cmd/scale"
	"k8s.io/kubectl/pkg/cmd/set"
	"k8s.io/kubectl/pkg/cmd/taint"
	"k8s.io/kubectl/pkg/cmd/top"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/cmd/version"
	"k8s.io/kubectl/pkg/cmd/wait"
	utilcomp "k8s.io/kubectl/pkg/util/completion"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/kubectl/pkg/util/term"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/kustomize"
)

const kubectlCmdHeaders = "KUBECTL_COMMAND_HEADERS"

type KubectlOptions struct {
	// 在 Kubernetes 中，PluginHandler 是一个接口，用于处理 kubectl 插件的功能。它提供了一种机制，允许开发人员编写自定义插件并将其集成到 kubectl 命令行工具中。
	PluginHandler PluginHandler
	// 表示命令行参数，是一个字符串数组，存储了传递给 kubectl 命令的参数。
	Arguments     []string
	// 这个结构体是用于获取 REST 客户端配置所需的一组值的集合，即用于配置 kubectl 客户端的一些参数和选项。
	// 这些字段用于配置 kubectl 客户端的行为，包括指定连接信息、认证信息、命名空间、请求超时等。它们提供了一种灵活的方式来自定义和配置 kubectl 客户端的行为。
	ConfigFlags   *genericclioptions.ConfigFlags
	// 表示输入输出流，是一个 genericiooptions.IOStreams 类型，用于处理命令的输入和输出。
	genericiooptions.IOStreams
}
/*
	定义了一个名为 defaultConfigFlags 的变量，它是一个 genericclioptions.ConfigFlags 类型的对象。
	defaultConfigFlags 是用于配置 Kubernetes 客户端的一组默认配置标志。在这段代码中，使用 genericclioptions.NewConfigFlags(true) 创建了一个新的 ConfigFlags 对象，并传递了一个布尔值 true 作为参数。
	接着，对这个 ConfigFlags 对象进行了一系列的配置操作：
	- WithDeprecatedPasswordFlag(): 添加了一个标志，用于处理已弃用的密码标志。
	- WithDiscoveryBurst(300): 设置了发现（discovery）的突发请求数为 300。这个配置用于控制客户端与 Kubernetes API 服务器之间的发现请求的突发性能。
	- WithDiscoveryQPS(50.0): 设置了发现（discovery）的每秒请求数为 50.0。这个配置用于控制客户端与 Kubernetes API 服务器之间的发现请求的每秒请求数。
	通过这些配置操作，defaultConfigFlags 变量成为了一个具有一组默认配置的 ConfigFlags 对象，可以在 Kubernetes 客户端中使用该对象来控制各种行为和参数。
*/
var defaultConfigFlags = genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag().WithDiscoveryBurst(300).WithDiscoveryQPS(50.0)

// NewDefaultKubectlCommand creates the `kubectl` command with default arguments
func NewDefaultKubectlCommand() *cobra.Command {
	return NewDefaultKubectlCommandWithArgs(KubectlOptions{
		// 使用 NewDefaultPluginHandler 函数创建一个默认的插件处理器，并将合法的插件文件名前缀作为参数传递给它。
		PluginHandler: NewDefaultPluginHandler(plugin.ValidPluginFilenamePrefixes),
		// 使用 os.Args 获取当前程序的命令行参数，并将其作为参数传递给 `kubectl` 命令。
		Arguments:     os.Args,
		ConfigFlags:   defaultConfigFlags,
		//  使用 genericiooptions.IOStreams 类型的对象，将标准输入、标准输出和标准错误输出与 `kubectl` 命令的输入、输出和错误输出进行绑定。
		IOStreams:     genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr},
	})
}

// NewDefaultKubectlCommandWithArgs creates the `kubectl` command with arguments
func NewDefaultKubectlCommandWithArgs(o KubectlOptions) *cobra.Command {
	cmd := NewKubectlCommand(o)

	if o.PluginHandler == nil {
		return cmd
	}

	if len(o.Arguments) > 1 {
		cmdPathPieces := o.Arguments[1:]

		// only look for suitable extension executables if
		// the specified command does not already exist
		if foundCmd, foundArgs, err := cmd.Find(cmdPathPieces); err != nil {
			// Also check the commands that will be added by Cobra.
			// These commands are only added once rootCmd.Execute() is called, so we
			// need to check them explicitly here.
			var cmdName string // first "non-flag" arguments
			for _, arg := range cmdPathPieces {
				if !strings.HasPrefix(arg, "-") {
					cmdName = arg
					break
				}
			}

			switch cmdName {
			case "help", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
				// Don't search for a plugin
			default:
				if err := HandlePluginCommand(o.PluginHandler, cmdPathPieces, false); err != nil {
					fmt.Fprintf(o.IOStreams.ErrOut, "Error: %v\n", err)
					os.Exit(1)
				}
			}
		} else if err == nil {
			if cmdutil.CmdPluginAsSubcommand.IsEnabled() {
				// Command exists(e.g. kubectl create), but it is not certain that
				// subcommand also exists (e.g. kubectl create networkpolicy)
				if IsSubcommandPluginAllowed(foundCmd.Name()) {
					var subcommand string
					for _, arg := range foundArgs { // first "non-flag" argument as subcommand
						if !strings.HasPrefix(arg, "-") {
							subcommand = arg
							break
						}
					}
					builtinSubcmdExist := false
					for _, subcmd := range foundCmd.Commands() {
						if subcmd.Name() == subcommand {
							builtinSubcmdExist = true
							break
						}
					}

					if !builtinSubcmdExist {
						if err := HandlePluginCommand(o.PluginHandler, cmdPathPieces, true); err != nil {
							fmt.Fprintf(o.IOStreams.ErrOut, "Error: %v\n", err)
							os.Exit(1)
						}
					}
				}
			}
		}
	}

	return cmd
}

// IsSubcommandPluginAllowed returns the given command is allowed
// to use plugin as subcommand if the subcommand does not exist as builtin.
func IsSubcommandPluginAllowed(foundCmd string) bool {
	allowedCmds := map[string]struct{}{"create": {}}
	_, ok := allowedCmds[foundCmd]
	return ok
}

// PluginHandler is capable of parsing command line arguments
// and performing executable filename lookups to search
// for valid plugin files, and execute found plugins.
type PluginHandler interface {
	// exists at the given filename, or a boolean false.
	// Lookup will iterate over a list of given prefixes
	// in order to recognize valid plugin filenames.
	// The first filepath to match a prefix is returned.
	Lookup(filename string) (string, bool)
	// Execute receives an executable's filepath, a slice
	// of arguments, and a slice of environment variables
	// to relay to the executable.
	Execute(executablePath string, cmdArgs, environment []string) error
}

/*
	DefaultPluginHandler 的结构体，它实现了 PluginHandler 接口。
	DefaultPluginHandler 结构体有一个字段 ValidPrefixes，它是一个字符串数组，用于存储合法的插件文件名前缀。
	DefaultPluginHandler 结构体的定义表明它是一个自定义的插件处理器，用于处理和管理 kubectl 插件。通过实现 PluginHandler 接口，DefaultPluginHandler 可以注册插件、执行插件和提供插件的帮助信息等功能。
	ValidPrefixes 字段用于定义合法的插件文件名前缀。这意味着只有以 ValidPrefixes 中指定的前缀开头的插件文件才会被认为是合法的插件文件。这种限制可以确保只有符合规范的插件文件才会被加载和执行。
	通过创建 DefaultPluginHandler 的实例，并设置合适的 ValidPrefixes 值，可以使用该插件处理器来管理和执行 kubectl 插件。
*/
// DefaultPluginHandler implements PluginHandler
type DefaultPluginHandler struct {
	ValidPrefixes []string
}

// NewDefaultPluginHandler instantiates the DefaultPluginHandler with a list of
// given filename prefixes used to identify valid plugin filenames.
func NewDefaultPluginHandler(validPrefixes []string) *DefaultPluginHandler {
	return &DefaultPluginHandler{
		ValidPrefixes: validPrefixes,
	}
}

// Lookup implements PluginHandler
func (h *DefaultPluginHandler) Lookup(filename string) (string, bool) {
	for _, prefix := range h.ValidPrefixes {
		path, err := exec.LookPath(fmt.Sprintf("%s-%s", prefix, filename))
		if shouldSkipOnLookPathErr(err) || len(path) == 0 {
			continue
		}
		return path, true
	}
	return "", false
}

func Command(name string, arg ...string) *exec.Cmd {
	cmd := &exec.Cmd{
		Path: name,
		Args: append([]string{name}, arg...),
	}
	if filepath.Base(name) == name {
		lp, err := exec.LookPath(name)
		if lp != "" && !shouldSkipOnLookPathErr(err) {
			// Update cmd.Path even if err is non-nil.
			// If err is ErrDot (especially on Windows), lp may include a resolved
			// extension (like .exe or .bat) that should be preserved.
			cmd.Path = lp
		}
	}
	return cmd
}

// Execute implements PluginHandler
func (h *DefaultPluginHandler) Execute(executablePath string, cmdArgs, environment []string) error {

	// Windows does not support exec syscall.
	if runtime.GOOS == "windows" {
		cmd := Command(executablePath, cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = environment
		err := cmd.Run()
		if err == nil {
			os.Exit(0)
		}
		return err
	}

	// invoke cmd binary relaying the environment and args given
	// append executablePath to cmdArgs, as execve will make first argument the "binary name".
	return syscall.Exec(executablePath, append([]string{executablePath}, cmdArgs...), environment)
}

// HandlePluginCommand receives a pluginHandler and command-line arguments and attempts to find
// a plugin executable on the PATH that satisfies the given arguments.
func HandlePluginCommand(pluginHandler PluginHandler, cmdArgs []string, exactMatch bool) error {
	var remainingArgs []string // all "non-flag" arguments
	for _, arg := range cmdArgs {
		if strings.HasPrefix(arg, "-") {
			break
		}
		remainingArgs = append(remainingArgs, strings.Replace(arg, "-", "_", -1))
	}

	if len(remainingArgs) == 0 {
		// the length of cmdArgs is at least 1
		return fmt.Errorf("flags cannot be placed before plugin name: %s", cmdArgs[0])
	}

	foundBinaryPath := ""

	// attempt to find binary, starting at longest possible name with given cmdArgs
	for len(remainingArgs) > 0 {
		path, found := pluginHandler.Lookup(strings.Join(remainingArgs, "-"))
		if !found {
			if exactMatch {
				// if exactMatch is true, we shouldn't continue searching with shorter names.
				// this is especially for not searching kubectl-create plugin
				// when kubectl-create-foo plugin is not found.
				break
			}
			remainingArgs = remainingArgs[:len(remainingArgs)-1]
			continue
		}

		foundBinaryPath = path
		break
	}

	if len(foundBinaryPath) == 0 {
		return nil
	}

	// invoke cmd binary relaying the current environment and args given
	if err := pluginHandler.Execute(foundBinaryPath, cmdArgs[len(remainingArgs):], os.Environ()); err != nil {
		return err
	}

	return nil
}

// NewKubectlCommand creates the `kubectl` command and its nested children.
func NewKubectlCommand(o KubectlOptions) *cobra.Command {
	/*
	代码在Kubernetes（k8s）中创建了一个警告处理程序。它使用`rest.NewWarningWriter`函数来创建一个警告写入器。
	`o.IOStreams.ErrOut`是一个错误输出流，用于将警告信息输出到终端。
	`rest.WarningWriterOptions`是一个结构体，用于配置警告写入器的选项。
	`Deduplicate`是一个布尔值，用于指定是否对重复的警告信息进行去重处理。如果设置为`true`，则相同的警告信息只会输出一次。
	`Color`是一个布尔值，用于指定是否在输出中使用颜色。它通过检查终端是否支持颜色输出来确定是否应该使用颜色。
   	因此，这段代码的含义是创建一个警告处理程序，将警告信息输出到错误输出流，并根据配置选项进行去重和颜色输出的处理。
	*/
	warningHandler := rest.NewWarningWriter(o.IOStreams.ErrOut, rest.WarningWriterOptions{Deduplicate: true, Color: term.AllowsColorOutput(o.IOStreams.ErrOut)})
	// 用户处理命令中的warnings-as-errors参数
	warningsAsErrors := false
	// Parent command to which all subcommands are added.
	cmds := &cobra.Command{
		// `Use`：用于设置命令的名称，这里设置为`kubectl`。
		Use:   "kubectl",
		// `Short`：用于设置命令的简短描述，这里使用了国际化（i18n）库的`T`函数来获取翻译后的字符串。
		Short: i18n.T("kubectl controls the Kubernetes cluster manager"),
		//  `Long`：用于设置命令的详细描述，这里使用了模板字符串来包含多行描述，并提供了一个URL链接作为更多信息的参考。
		Long: templates.LongDesc(`
      kubectl controls the Kubernetes cluster manager.

      Find more information at:
            https://kubernetes.io/docs/reference/kubectl/`),
	  	// `Run`：用于设置命令的主要执行函数，这里设置为`runHelp`，即运行帮助命令。
        Run: runHelp,
		// Hook before and after Run initialize and write profiles to disk,
		// respectively.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// 设置了默认的警告处理程序为`warningHandler`
			rest.SetDefaultWarningHandler(warningHandler)
			// 检查命令的名称是否为`cobra.ShellCompRequestCmd`，如果是，表示请求了Shell自动补全，会调用`plugin.SetupPluginCompletion`函数进行插件自动补全的设置。
			if cmd.Name() == cobra.ShellCompRequestCmd {
				// This is the __complete or __completeNoDesc command which
				// indicates shell completion has been requested.
				plugin.SetupPluginCompletion(cmd, args)
			}
			// 调用一个名为`initProfiling`的函数，用于初始化性能分析（profiling）。
			return initProfiling()
		},
		PersistentPostRunE: func(*cobra.Command, []string) error {
			// 这段代码定义了一个名为`flushProfiling`的函数，用于刷新性能分析数据。
			if err := flushProfiling(); err != nil {
				return err
			}
			if warningsAsErrors {
				count := warningHandler.WarningCount()
				switch count {
				case 0:
					// no warnings
				case 1:
					return fmt.Errorf("%d warning received", count)
				default:
					return fmt.Errorf("%d warnings received", count)
				}
			}
			return nil
		},
	}
	// From this point and forward we get warnings on flags that contain "_" separators
	// when adding them with hyphen instead of the original name.
	cmds.SetGlobalNormalizationFunc(cliflag.WarnWordSepNormalizeFunc)

	flags := cmds.PersistentFlags()

	addProfilingFlags(flags)

	flags.BoolVar(&warningsAsErrors, "warnings-as-errors", warningsAsErrors, "Treat warnings received from the server as errors and exit with a non-zero exit code")

	kubeConfigFlags := o.ConfigFlags
	if kubeConfigFlags == nil {
		kubeConfigFlags = defaultConfigFlags
	}
	kubeConfigFlags.AddFlags(flags)
	matchVersionKubeConfigFlags := cmdutil.NewMatchVersionFlags(kubeConfigFlags)
	matchVersionKubeConfigFlags.AddFlags(flags)
	// Updates hooks to add kubectl command headers: SIG CLI KEP 859.
	addCmdHeaderHooks(cmds, kubeConfigFlags)

	f := cmdutil.NewFactory(matchVersionKubeConfigFlags)

	/*
		如下代码用于在运行代理命令之前清除WrapConfigFn配置。
		在Kubernetes中，代理命令与CommandHeaderRoundTripper不兼容，因此需要在运行代理命令之前清除WrapConfigFn配置。
		【因为在CommandHeaderRoundTripper中可以自定义请求头，但是如果使用了代理，那么没有办法把这个自定义的请求传递到kube apiserver，因为经过了代理】
		首先，代码创建了一个proxyCmd对象，该对象是通过调用proxy.NewCmdProxy函数创建的。该函数接受一个cmdutil.Factory对象f和o.IOStreams对象作为参数，用于创建代理命令。
		接下来，代码设置了proxyCmd对象的PreRun字段，该字段是一个函数，在运行代理命令之前执行。在PreRun函数中，将kubeConfigFlags.WrapConfigFn配置设置为nil，即清除该配置。
		通过清除WrapConfigFn配置，确保在运行代理命令时不会使用CommandHeaderRoundTripper，以避免不兼容的问题。
	*/
	// Proxy command is incompatible with CommandHeaderRoundTripper, so
	// clear the WrapConfigFn before running proxy command.
	proxyCmd := proxy.NewCmdProxy(f, o.IOStreams)
	proxyCmd.PreRun = func(cmd *cobra.Command, args []string) {
		kubeConfigFlags.WrapConfigFn = nil
	}

	// 代码调用get.NewCmdGet函数创建一个用于执行"get"操作的命令对象getCmd。该函数接受三个参数："kubectl"表示父级资源名称，f表示cmdutil.Factory对象，o.IOStreams表示genericiooptions.IOStreams对象。
	// Avoid import cycle by setting ValidArgsFunction here instead of in NewCmdGet()
	getCmd := get.NewCmdGet("kubectl", f, o.IOStreams)
	// 接下来，代码设置getCmd对象的ValidArgsFunction属性。ValidArgsFunction属性是一个函数，用于提供有效参数的自动补全功能。
	// 在这里，通过调用utilcomp.ResourceTypeAndNameCompletionFunc函数，并将cmdutil.Factory对象f作为参数传入，设置ValidArgsFunction属性。
	getCmd.ValidArgsFunction = utilcomp.ResourceTypeAndNameCompletionFunc(f)
	// 把一组子命令添加到kubectl中
	groups := templates.CommandGroups{
		{
			Message: "Basic Commands (Beginner):",
			Commands: []*cobra.Command{
				create.NewCmdCreate(f, o.IOStreams),
				expose.NewCmdExposeService(f, o.IOStreams),
				run.NewCmdRun(f, o.IOStreams),
				set.NewCmdSet(f, o.IOStreams),
			},
		},
		{
			Message: "Basic Commands (Intermediate):",
			Commands: []*cobra.Command{
				explain.NewCmdExplain("kubectl", f, o.IOStreams),
				getCmd,
				edit.NewCmdEdit(f, o.IOStreams),
				delete.NewCmdDelete(f, o.IOStreams),
			},
		},
		{
			Message: "Deploy Commands:",
			Commands: []*cobra.Command{
				rollout.NewCmdRollout(f, o.IOStreams),
				scale.NewCmdScale(f, o.IOStreams),
				autoscale.NewCmdAutoscale(f, o.IOStreams),
			},
		},
		{
			Message: "Cluster Management Commands:",
			Commands: []*cobra.Command{
				certificates.NewCmdCertificate(f, o.IOStreams),
				clusterinfo.NewCmdClusterInfo(f, o.IOStreams),
				top.NewCmdTop(f, o.IOStreams),
				drain.NewCmdCordon(f, o.IOStreams),
				drain.NewCmdUncordon(f, o.IOStreams),
				drain.NewCmdDrain(f, o.IOStreams),
				taint.NewCmdTaint(f, o.IOStreams),
			},
		},
		{
			Message: "Troubleshooting and Debugging Commands:",
			Commands: []*cobra.Command{
				describe.NewCmdDescribe("kubectl", f, o.IOStreams),
				logs.NewCmdLogs(f, o.IOStreams),
				attach.NewCmdAttach(f, o.IOStreams),
				cmdexec.NewCmdExec(f, o.IOStreams),
				portforward.NewCmdPortForward(f, o.IOStreams),
				proxyCmd,
				cp.NewCmdCp(f, o.IOStreams),
				auth.NewCmdAuth(f, o.IOStreams),
				debug.NewCmdDebug(f, o.IOStreams),
				events.NewCmdEvents(f, o.IOStreams),
			},
		},
		{
			Message: "Advanced Commands:",
			Commands: []*cobra.Command{
				diff.NewCmdDiff(f, o.IOStreams),
				apply.NewCmdApply("kubectl", f, o.IOStreams),
				patch.NewCmdPatch(f, o.IOStreams),
				replace.NewCmdReplace(f, o.IOStreams),
				wait.NewCmdWait(f, o.IOStreams),
				kustomize.NewCmdKustomize(o.IOStreams),
			},
		},
		{
			Message: "Settings Commands:",
			Commands: []*cobra.Command{
				label.NewCmdLabel(f, o.IOStreams),
				annotate.NewCmdAnnotate("kubectl", f, o.IOStreams),
				completion.NewCmdCompletion(o.IOStreams.Out, ""),
			},
		},
	}
	groups.Add(cmds)

	filters := []string{"options"}

	// kubectl alpha命令
	// Hide the "alpha" subcommand if there are no alpha commands in this build.
	alpha := NewCmdAlpha(f, o.IOStreams)
	if !alpha.HasSubCommands() {
		filters = append(filters, alpha.Name())
	}

	templates.ActsAsRootCommand(cmds, filters, groups...)

	utilcomp.SetFactoryForCompletion(f)
	registerCompletionFuncForGlobalFlags(cmds, f)
	// kubectl alpha命令
	cmds.AddCommand(alpha)
	// kubectl config命令
	cmds.AddCommand(cmdconfig.NewCmdConfig(clientcmd.NewDefaultPathOptions(), o.IOStreams))
	// kubectl plugin命令
	cmds.AddCommand(plugin.NewCmdPlugin(o.IOStreams))
	// kubectl version命令
	cmds.AddCommand(version.NewCmdVersion(f, o.IOStreams))
	// kubectl api-versions命令
	cmds.AddCommand(apiresources.NewCmdAPIVersions(f, o.IOStreams))
	// kubectl api-resources命令
	cmds.AddCommand(apiresources.NewCmdAPIResources(f, o.IOStreams))
	// kubectl options 命令
	cmds.AddCommand(options.NewCmdOptions(o.IOStreams.Out))

	// Stop warning about normalization of flags. That makes it possible to
	// add the klog flags later.
	cmds.SetGlobalNormalizationFunc(cliflag.WordSepNormalizeFunc)
	return cmds
}

// addCmdHeaderHooks performs updates on two hooks:
//  1. Modifies the passed "cmds" persistent pre-run function to parse command headers.
//     These headers will be subsequently added as X-headers to every
//     REST call.
//  2. Adds CommandHeaderRoundTripper as a wrapper around the standard
//     RoundTripper. CommandHeaderRoundTripper adds X-Headers then delegates
//     to standard RoundTripper.
//
// For beta, these hooks are updated unless the KUBECTL_COMMAND_HEADERS environment variable
// is set, and the value of the env var is false (or zero).
// See SIG CLI KEP 859 for more information:
//
//	https://github.com/kubernetes/enhancements/tree/master/keps/sig-cli/859-kubectl-headers
func addCmdHeaderHooks(cmds *cobra.Command, kubeConfigFlags *genericclioptions.ConfigFlags) {
	// If the feature gate env var is set to "false", then do no add kubectl command headers.
	if value, exists := os.LookupEnv(kubectlCmdHeaders); exists {
		if value == "false" || value == "0" {
			klog.V(5).Infoln("kubectl command headers turned off")
			return
		}
	}
	klog.V(5).Infoln("kubectl command headers turned on")
	crt := &genericclioptions.CommandHeaderRoundTripper{}
	existingPreRunE := cmds.PersistentPreRunE
	// Add command parsing to the existing persistent pre-run function.
	cmds.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		crt.ParseCommandHeaders(cmd, args)
		return existingPreRunE(cmd, args)
	}
	wrapConfigFn := kubeConfigFlags.WrapConfigFn
	// Wraps CommandHeaderRoundTripper around standard RoundTripper.
	kubeConfigFlags.WrapConfigFn = func(c *rest.Config) *rest.Config {
		if wrapConfigFn != nil {
			c = wrapConfigFn(c)
		}
		c.Wrap(func(rt http.RoundTripper) http.RoundTripper {
			// Must be separate RoundTripper; not "crt" closure.
			// Fixes: https://github.com/kubernetes/kubectl/issues/1098
			return &genericclioptions.CommandHeaderRoundTripper{
				Delegate: rt,
				Headers:  crt.Headers,
			}
		})
		return c
	}
}

func runHelp(cmd *cobra.Command, args []string) {
	cmd.Help()
}
/*
	如下代码定义了一个函数registerCompletionFuncForGlobalFlags，用于为全局标志选项注册自动补全函数。
	函数接受一个*cobra.Command对象cmd和一个cmdutil.Factory对象f作为参数。
	函数内部通过调用cmd.RegisterFlagCompletionFunc函数为不同的标志选项注册自动补全函数。
	对于"namespace"标志选项，注册的自动补全函数会调用utilcomp.CompGetResource函数，根据toComplete参数获取与toComplete前缀匹配的命名空间名称列表，并返回结果和一个指示不进行文件补全的cobra.ShellCompDirective枚举值。
	对于"context"标志选项，注册的自动补全函数会调用utilcomp.ListContextsInConfig函数，根据toComplete参数获取与toComplete前缀匹配的上下文名称列表，并返回结果和一个指示不进行文件补全的cobra.ShellCompDirective枚举值。
	对于"cluster"标志选项，注册的自动补全函数会调用utilcomp.ListClustersInConfig函数，根据toComplete参数获取与toComplete前缀匹配的集群名称列表，并返回结果和一个指示不进行文件补全的cobra.ShellCompDirective枚举值。
	对于"user"标志选项，注册的自动补全函数会调用utilcomp.ListUsersInConfig函数，根据toComplete参数获取与toComplete前缀匹配的用户名称列表，并返回结果和一个指示不进行文件补全的cobra.ShellCompDirective枚举值。
	最后，使用cmdutil.CheckErr函数检查是否有错误发生。
*/
func registerCompletionFuncForGlobalFlags(cmd *cobra.Command, f cmdutil.Factory) {
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"namespace",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.CompGetResource(f, "namespace", toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"context",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.ListContextsInConfig(toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"cluster",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.ListClustersInConfig(toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"user",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.ListUsersInConfig(toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
}
