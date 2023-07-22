/*
Copyright 2021 The Kubernetes Authors.

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
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const (
	kubectlCommandHeader = "Kubectl-Command"
	kubectlSessionHeader = "Kubectl-Session"
)
/*
	这段代码定义了一个名为CommandHeaderRoundTripper的结构体。CommandHeaderRoundTripper是一个包装器，
	用于在标准的RoundTripper周围添加一层，以在委托之前添加请求头。它实现了Go标准库的"http.RoundTripper"接口。
	CommandHeaderRoundTripper结构体包含以下字段：
	- Delegate：一个http.RoundTripper类型的字段，表示要委托的标准RoundTripper。该字段用于处理实际的HTTP请求和响应。
	- Headers：一个map[string]string类型的字段，表示要添加到请求中的头部信息。该字段允许用户指定自定义的请求头，例如命令头等。
	CommandHeaderRoundTripper结构体的目的是为了在标准的RoundTripper之前添加自定义的请求头。通过实现http.RoundTripper接口，
	可以将CommandHeaderRoundTripper作为HTTP客户端的一部分，以在发送请求之前自动添加请求头。这样可以实现在HTTP请求过程中自动添加自定义头部的功能。
*/
// CommandHeaderRoundTripper adds a layer around the standard
// round tripper to add Request headers before delegation. Implements
// the go standard library "http.RoundTripper" interface.
type CommandHeaderRoundTripper struct {
	Delegate http.RoundTripper
	Headers  map[string]string
}

// CommandHeaderRoundTripper adds Request headers before delegating to standard
// round tripper. These headers are kubectl command headers which
// detail the kubectl command. See SIG CLI KEP 859:
//
//	https://github.com/kubernetes/enhancements/tree/master/keps/sig-cli/859-kubectl-headers
func (c *CommandHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for header, value := range c.Headers {
		req.Header.Set(header, value)
	}
	return c.Delegate.RoundTrip(req)
}

// ParseCommandHeaders fills in a map of custom headers into the CommandHeaderRoundTripper. These
// headers are then filled into each request. For details on the custom headers see:
//
//	https://github.com/kubernetes/enhancements/tree/master/keps/sig-cli/859-kubectl-headers
//
// Each call overwrites the previously parsed command headers (not additive).
// TODO(seans3): Parse/add flags removing PII from flag values.
func (c *CommandHeaderRoundTripper) ParseCommandHeaders(cmd *cobra.Command, args []string) {
	if cmd == nil {
		return
	}
	// Overwrites previously parsed command headers (headers not additive).
	c.Headers = map[string]string{}
	// Session identifier to aggregate multiple Requests from single kubectl command.
	uid := uuid.New().String()
	c.Headers[kubectlSessionHeader] = uid
	// Iterate up the hierarchy of commands from the leaf command to create
	// the full command string. Example: kubectl create secret generic
	cmdStrs := []string{}
	for cmd.HasParent() {
		parent := cmd.Parent()
		currName := strings.TrimSpace(cmd.Name())
		cmdStrs = append([]string{currName}, cmdStrs...)
		cmd = parent
	}
	currName := strings.TrimSpace(cmd.Name())
	cmdStrs = append([]string{currName}, cmdStrs...)
	if len(cmdStrs) > 0 {
		c.Headers[kubectlCommandHeader] = strings.Join(cmdStrs, " ")
	}
}

// CancelRequest is propagated to the Delegate RoundTripper within
// if the wrapped RoundTripper implements this function.
func (c *CommandHeaderRoundTripper) CancelRequest(req *http.Request) {
	type canceler interface {
		CancelRequest(*http.Request)
	}
	// If possible, call "CancelRequest" on the wrapped Delegate RoundTripper.
	if cr, ok := c.Delegate.(canceler); ok {
		cr.CancelRequest(req)
	}
}
