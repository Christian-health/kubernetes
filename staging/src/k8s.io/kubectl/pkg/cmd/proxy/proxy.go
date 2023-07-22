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

package proxy

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/proxy"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)
/*
如下代码定义了一个名为ProxyOptions的结构体。ProxyOptions结构体包含了执行代理操作所需的数据。
	ProxyOptions结构体包含以下字段：
	- staticDir：一个字符串类型的字段，表示静态文件目录。
	- staticPrefix：一个字符串类型的字段，表示静态文件的前缀。
	- apiPrefix：一个字符串类型的字段，表示API的前缀。
	- acceptPaths：一个字符串类型的字段，表示要接受的路径。
	- rejectPaths：一个字符串类型的字段，表示要拒绝的路径。
	- acceptHosts：一个字符串类型的字段，表示要接受的主机。
	- rejectMethods：一个字符串类型的字段，表示要拒绝的HTTP方法。
	- port：一个整数类型的字段，表示代理的端口。
	- address：一个字符串类型的字段，表示代理的地址。
	- disableFilter：一个布尔类型的字段，表示是否禁用过滤器。
	- unixSocket：一个字符串类型的字段，表示Unix套接字。
	- keepalive：一个time.Duration类型的字段，表示保持活动连接的时间。
	- appendServerPath：一个布尔类型的字段，表示是否附加服务器路径。
	- clientConfig：一个rest.Config类型的字段，表示客户端配置。
	- filter：一个proxy.FilterServer类型的字段，表示过滤器服务器。
	- genericiooptions.IOStreams：一个genericiooptions.IOStreams类型的字段，表示输入/输出流。
ProxyOptions结构体用于存储执行代理操作所需的各种选项和配置。这些选项和配置包括静态文件目录、API前缀、路径过滤、端口、地址、客户端配置等。通过使用ProxyOptions结构体，可以方便地传递和管理代理操作所需的参数和设置。
*/
// ProxyOptions have the data required to perform the proxy operation
type ProxyOptions struct {
	// Common user flags
	staticDir     string
	staticPrefix  string
	apiPrefix     string
	acceptPaths   string
	rejectPaths   string
	acceptHosts   string
	rejectMethods string
	port          int
	address       string
	disableFilter bool
	unixSocket    string
	keepalive     time.Duration

	appendServerPath bool

	clientConfig *rest.Config
	filter       *proxy.FilterServer

	genericiooptions.IOStreams
}

const (
	defaultPort         = 8001
	defaultStaticPrefix = "/static/"
	defaultAPIPrefix    = "/"
	defaultAddress      = "127.0.0.1"
)

var (
	proxyLong = templates.LongDesc(i18n.T(`
		Creates a proxy server or application-level gateway between localhost and
		the Kubernetes API server. It also allows serving static content over specified
		HTTP path. All incoming data enters through one port and gets forwarded to
		the remote Kubernetes API server port, except for the path matching the static content path.`))

	proxyExample = templates.Examples(i18n.T(`
		# To proxy all of the Kubernetes API and nothing else
		kubectl proxy --api-prefix=/

		# To proxy only part of the Kubernetes API and also some static files
		# You can get pods info with 'curl localhost:8001/api/v1/pods'
		kubectl proxy --www=/my/files --www-prefix=/static/ --api-prefix=/api/

		# To proxy the entire Kubernetes API at a different root
		# You can get pods info with 'curl localhost:8001/custom/api/v1/pods'
		kubectl proxy --api-prefix=/custom/

		# Run a proxy to the Kubernetes API server on port 8011, serving static content from ./local/www/
		kubectl proxy --port=8011 --www=./local/www/

		# Run a proxy to the Kubernetes API server on an arbitrary local port
		# The chosen port for the server will be output to stdout
		kubectl proxy --port=0

		# Run a proxy to the Kubernetes API server, changing the API prefix to k8s-api
		# This makes e.g. the pods API available at localhost:8001/k8s-api/v1/pods/
		kubectl proxy --api-prefix=/k8s-api`))
)

// NewProxyOptions creates the options for proxy
func NewProxyOptions(ioStreams genericiooptions.IOStreams) *ProxyOptions {
	return &ProxyOptions{
		IOStreams:     ioStreams,
		staticPrefix:  defaultStaticPrefix,
		apiPrefix:     defaultAPIPrefix,
		acceptPaths:   proxy.DefaultPathAcceptRE,
		rejectPaths:   proxy.DefaultPathRejectRE,
		acceptHosts:   proxy.DefaultHostAcceptRE,
		rejectMethods: proxy.DefaultMethodRejectRE,
		port:          defaultPort,
		address:       defaultAddress,
	}
}

// NewCmdProxy returns the proxy Cobra command
func NewCmdProxy(f cmdutil.Factory, ioStreams genericiooptions.IOStreams) *cobra.Command {
	o := NewProxyOptions(ioStreams)

	cmd := &cobra.Command{
		Use:                   "proxy [--port=PORT] [--www=static-dir] [--www-prefix=prefix] [--api-prefix=prefix]",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Run a proxy to the Kubernetes API server"),
		Long:                  proxyLong,
		Example:               proxyExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.RunProxy())
		},
	}
	// 解析命令传递的各种参数，使用这些值初始化ProxyOptions结构
	cmd.Flags().StringVarP(&o.staticDir, "www", "w", o.staticDir, "Also serve static files from the given directory under the specified prefix.")
	cmd.Flags().StringVarP(&o.staticPrefix, "www-prefix", "P", o.staticPrefix, "Prefix to serve static files under, if static file directory is specified.")
	cmd.Flags().StringVar(&o.apiPrefix, "api-prefix", o.apiPrefix, "Prefix to serve the proxied API under.")
	cmd.Flags().StringVar(&o.acceptPaths, "accept-paths", o.acceptPaths, "Regular expression for paths that the proxy should accept.")
	cmd.Flags().StringVar(&o.rejectPaths, "reject-paths", o.rejectPaths, "Regular expression for paths that the proxy should reject. Paths specified here will be rejected even accepted by --accept-paths.")
	cmd.Flags().StringVar(&o.acceptHosts, "accept-hosts", o.acceptHosts, "Regular expression for hosts that the proxy should accept.")
	cmd.Flags().StringVar(&o.rejectMethods, "reject-methods", o.rejectMethods, "Regular expression for HTTP methods that the proxy should reject (example --reject-methods='POST,PUT,PATCH'). ")
	cmd.Flags().IntVarP(&o.port, "port", "p", o.port, "The port on which to run the proxy. Set to 0 to pick a random port.")
	cmd.Flags().StringVar(&o.address, "address", o.address, "The IP address on which to serve on.")
	cmd.Flags().BoolVar(&o.disableFilter, "disable-filter", o.disableFilter, "If true, disable request filtering in the proxy. This is dangerous, and can leave you vulnerable to XSRF attacks, when used with an accessible port.")
	cmd.Flags().StringVarP(&o.unixSocket, "unix-socket", "u", o.unixSocket, "Unix socket on which to run the proxy.")
	cmd.Flags().DurationVar(&o.keepalive, "keepalive", o.keepalive, "keepalive specifies the keep-alive period for an active network connection. Set to 0 to disable keepalive.")
	cmd.Flags().BoolVar(&o.appendServerPath, "append-server-path", o.appendServerPath, "If true, enables automatic path appending of the kube context server path to each request.")
	return cmd
}
/*
	ProxyOptions结构体的Complete方法。
	该方法用于根据命令行参数和工厂对象的数据，完成ProxyOptions结构体的初始化。
*/
// Complete adapts from the command line args and factory to the data required.
func (o *ProxyOptions) Complete(f cmdutil.Factory) error {
	// 首先通过调用cmdutil.Factory对象的ToRESTConfig方法，将工厂对象转换为rest.Config类型的客户端配置。转换过程中，还会检查是否存在错误，如果有错误则返回该错误。
	clientConfig, err := f.ToRESTConfig()
	if err != nil {
		return err
	}
	o.clientConfig = clientConfig
	// 	接下来，方法会对staticPrefix和apiPrefix字段进行处理。如果它们不以斜杠结尾，方法会自动在末尾添加斜杠。
	if !strings.HasSuffix(o.staticPrefix, "/") {
		o.staticPrefix += "/"
	}

	if !strings.HasSuffix(o.apiPrefix, "/") {
		o.apiPrefix += "/"
	}
	/*
		然后，方法会根据appendServerPath字段的值来判断是否需要自动添加服务器路径。如果appendServerPath为false，
		则会解析clientConfig.Host字段，并检查其中是否包含服务器路径。如果存在服务器路径，并且不是根路径(/)，则会发出警告提示。
	*/
	if o.appendServerPath == false {
		target, err := url.Parse(clientConfig.Host)
		if err != nil {
			return err
		}
		if target.Path != "" && target.Path != "/" {
			klog.Warning("Your kube context contains a server path " + target.Path + ", use --append-server-path to automatically append the path to each request")
		}
	}
	/*
		接下来，方法会根据disableFilter字段的值来判断是否禁用请求过滤器。
		如果禁用过滤器，并且unixSocket字段为空，则会发出警告提示，并将filter字段设置为nil。
		否则，将根据acceptPaths、rejectPaths、acceptHosts和rejectMethods字段的值创建一个proxy.FilterServer对象，并将其赋值给filter字段。
	*/
	if o.disableFilter {
		if o.unixSocket == "" {
			klog.Warning("Request filter disabled, your proxy is vulnerable to XSRF attacks, please be cautious")
		}
		o.filter = nil
	} else {
		o.filter = &proxy.FilterServer{
			AcceptPaths:   proxy.MakeRegexpArrayOrDie(o.acceptPaths),
			RejectPaths:   proxy.MakeRegexpArrayOrDie(o.rejectPaths),
			AcceptHosts:   proxy.MakeRegexpArrayOrDie(o.acceptHosts),
			RejectMethods: proxy.MakeRegexpArrayOrDie(o.rejectMethods),
		}
	}
	// 最后，方法返回nil，表示Complete方法执行成功。如果在执行过程中发生任何错误，将返回相应的错误信息。
	return nil
}
/*
	如下代码是ProxyOptions结构体的Validate方法。该方法用于检查ProxyOptions结构体中的选项，以确定是否有足够的信息来运行命令。
*/
// Validate checks to the ProxyOptions to see if there is sufficient information to run the command.
func (o ProxyOptions) Validate() error {
	// 方法首先检查port和unixSocket字段的值。如果port字段不等于默认端口值（defaultPort），并且unixSocket字段不为空，则会返回一个错误，提示不能同时设置--unix-socket和--port。
	if o.port != defaultPort && o.unixSocket != "" {
		return errors.New("cannot set --unix-socket and --port at the same time")
	}
	// 	接下来，方法检查staticDir字段的值。如果staticDir字段不为空，则会尝试获取该目录的文件信息。如果获取文件信息时发生错误，则会发出警告提示。如果获取的文件信息不是目录，则会发出警告提示。
	if o.staticDir != "" {
		fileInfo, err := os.Stat(o.staticDir)
		if err != nil {
			klog.Warning("Failed to stat static file directory "+o.staticDir+": ", err)
		} else if !fileInfo.IsDir() {
			klog.Warning("Static file directory " + o.staticDir + " is not a directory")
		}
	}
	// 最后，方法返回nil，表示Validate方法执行成功。如果在执行过程中发现任何问题，将返回相应的错误信息。
	return nil
}
/*
	如下代码是ProxyOptions结构体的RunProxy方法。该方法用于检查给定的参数，并执行相应的命令。
*/
// RunProxy checks given arguments and executes command
func (o ProxyOptions) RunProxy() error {
	// 方法首先根据ProxyOptions结构体中的选项创建一个proxy.Server对象，并传入相应的参数。如果在创建过程中发生错误，则会返回该错误。
	server, err := proxy.NewServer(o.staticDir, o.apiPrefix, o.staticPrefix, o.filter, o.clientConfig, o.keepalive, o.appendServerPath)

	if err != nil {
		return err
	}
	/*
		接下来，方法根据unixSocket字段的值来选择监听方式。如果unixSocket为空，则会调用server对象的Listen方法，传入address和port参数，以监听TCP连接。
		如果unixSocket不为空，则会调用server对象的ListenUnix方法，传入unixSocket参数，以监听Unix域套接字连接。
	*/
	// Separate listening from serving so we can report the bound port
	// when it is chosen by os (eg: port == 0)
	var l net.Listener
	if o.unixSocket == "" {
		l, err = server.Listen(o.address, o.port)
	} else {
		l, err = server.ListenUnix(o.unixSocket)
	}
	if err != nil {
		return err
	}
	// 	然后，方法会将监听的地址信息输出到o.IOStreams.Out流中。
	fmt.Fprintf(o.IOStreams.Out, "Starting to serve on %s\n", l.Addr().String())
	// 最后，方法调用server对象的ServeOnListener方法，传入监听器l，以开始提供服务。如果在服务过程中发生错误，则会返回该错误。
	return server.ServeOnListener(l)
}
