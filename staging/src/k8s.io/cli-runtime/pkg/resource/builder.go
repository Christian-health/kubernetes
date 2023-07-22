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

package resource

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured/unstructuredscheme"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

var FileExtensions = []string{".json", ".yaml", ".yml"}
var InputExtensions = append(FileExtensions, "stdin")

const defaultHttpGetAttempts = 3
const pathNotExistError = "the path %q does not exist"
/*
	如下代码定义了Builder结构体，用于提供方便的功能，将命令行参数和参数转换为要使用Visitor接口迭代的资源列表。
	Builder结构体提供了一些便捷的方法，用于从命令行参数和参数中提取资源，并将其转换为要使用Visitor接口迭代的资源列表。它还包含了一些与资源处理相关的字段和方法，用于方便地处理和操作资源。
	总的来说，Builder结构体用于提供方便的功能，将命令行参数和参数转换为要使用Visitor接口迭代的资源列表，并提供了一些与资源处理相关的字段和方法。它可以方便地处理和操作资源，并提供了一些便捷的方法来处理和操作资源。
*/
// Builder provides convenience functions for taking arguments and parameters
// from the command line and converting them to a list of resources to iterate
// over using the Visitor interface.
type Builder struct {
	//	- categoryExpanderFn：CategoryExpanderFunc类型的函数，用于扩展资源类别。
	categoryExpanderFn CategoryExpanderFunc
	//	- mapper：mapper类型的对象，用于映射对象。
	// mapper is set explicitly by resource builders
	mapper *mapper
	//  - clientConfigFn：ClientConfigFunc类型的函数，用于生成客户端配置。
	// clientConfigFn is a function to produce a client, *if* you need one
	clientConfigFn ClientConfigFunc
	// - restMapperFn：RESTMapperFunc类型的函数，用于将资源和API组版本映射到RESTMapper。
	restMapperFn RESTMapperFunc
	// - objectTyper：runtime.ObjectTyper类型的对象，用于确定对象类型。
	// objectTyper is statically determinant per-command invocation based on your internal or unstructured choice
	// it does not ever need to rely upon discovery.
	objectTyper runtime.ObjectTyper
	//  - negotiatedSerializer：runtime.NegotiatedSerializer类型的对象，用于序列化和反序列化对象。
	// codecFactory describes which codecs you want to use
	negotiatedSerializer runtime.NegotiatedSerializer
	//  - local：标志是否只能在本地执行，不能进行服务器调用。
	// local indicates that we cannot make server calls
	local bool
	//  - errs：存储错误信息的切片。
	errs []error
	//	- paths：存储Visitor对象的切片，用于迭代资源。
	paths      []Visitor
	//  - stream：标志是否使用流式处理。
	stream     bool
	//	- stdinInUse：标志是否正在使用标准输入。
	stdinInUse bool
	//	- dir：标志是否处理目录。
	dir        bool
	//	- visitorConcurrency：Visitor并发数。
	visitorConcurrency int
	//	- labelSelector：标签选择器。
	labelSelector     *string
	//	- fieldSelector：字段选择器。
	fieldSelector     *string
	//	- selectAll：标志是否选择所有资源。
	selectAll         bool
	//	- limitChunks：限制处理的资源块数量。
	limitChunks       int64
	//   - requestTransforms：请求转换列表。
	requestTransforms []RequestTransform
	//   - resources：资源列表。
	resources   []string
	//   - subresource：子资源。
	subresource string
	//   - namespace：命名空间。
	namespace    string
	//	- allNamespace：标志是否处理所有命名空间。
	allNamespace bool
	//	- names：名称列表。
	names        []string
	//	- resourceTuples：资源元组列表。
	resourceTuples []resourceTuple
	//	- defaultNamespace：标志是否使用默认命名空间。
	defaultNamespace bool
	//	- requireNamespace：标志是否要求命名空间。
	requireNamespace bool
	//	- flatten：标志是否展平结果。
	flatten bool

	//	- latest：标志是否使用最新版本。
	latest  bool
	//  - requireObject：标志是否需要对象。
	requireObject bool

	//  - singleResourceType：标志是否只有一个资源类型。
	singleResourceType bool

	//  - continueOnError：标志是否在错误发生时继续执行。
	continueOnError    bool
	//  - singleItemImplied：标志是否隐含只有一个资源。
	singleItemImplied bool

	//   - schema：ContentValidator类型的对象，用于验证内容。
	schema ContentValidator

	//   - fakeClientFn：用于测试的FakeClientFunc类型的函数。
	// fakeClientFn is used for testing
	fakeClientFn FakeClientFunc
}

var missingResourceError = fmt.Errorf(`You must provide one or more resources by argument or filename.
Example resource specifications include:
   '-f rsrc.yaml'
   '--filename=rsrc.json'
   '<resource> <name>'
   '<resource>'`)

var LocalResourceError = errors.New(`error: you must specify resources by --filename when --local is set.
Example resource specifications include:
   '-f rsrc.yaml'
   '--filename=rsrc.json'`)

var StdinMultiUseError = errors.New("standard input cannot be used for multiple arguments")

// TODO: expand this to include other errors.
func IsUsageError(err error) bool {
	if err == nil {
		return false
	}
	return err == missingResourceError
}

type FilenameOptions struct {
	Filenames []string
	Kustomize string
	Recursive bool
}

func (o *FilenameOptions) validate() []error {
	var errs []error
	if len(o.Filenames) > 0 && len(o.Kustomize) > 0 {
		errs = append(errs, fmt.Errorf("only one of -f or -k can be specified"))
	}
	if len(o.Kustomize) > 0 && o.Recursive {
		errs = append(errs, fmt.Errorf("the -k flag can't be used with -f or -R"))
	}
	return errs
}

func (o *FilenameOptions) RequireFilenameOrKustomize() error {
	if len(o.Filenames) == 0 && len(o.Kustomize) == 0 {
		return fmt.Errorf("must specify one of -f and -k")
	}
	return nil
}

type resourceTuple struct {
	Resource string
	Name     string
}

type FakeClientFunc func(version schema.GroupVersion) (RESTClient, error)

func NewFakeBuilder(fakeClientFn FakeClientFunc, restMapper RESTMapperFunc, categoryExpander CategoryExpanderFunc) *Builder {
	ret := newBuilder(nil, restMapper, categoryExpander)
	ret.fakeClientFn = fakeClientFn
	return ret
}

// NewBuilder creates a builder that operates on generic objects. At least one of
// internal or unstructured must be specified.
// TODO: Add versioned client (although versioned is still lossy)
// TODO remove internal and unstructured mapper and instead have them set the negotiated serializer for use in the client
func newBuilder(clientConfigFn ClientConfigFunc, restMapper RESTMapperFunc, categoryExpander CategoryExpanderFunc) *Builder {
	return &Builder{
		clientConfigFn:     clientConfigFn,
		restMapperFn:       restMapper,
		categoryExpanderFn: categoryExpander,
		requireObject:      true,
	}
}

// noopClientGetter implements RESTClientGetter returning only errors.
// used as a dummy getter in a local-only builder.
type noopClientGetter struct{}

func (noopClientGetter) ToRESTConfig() (*rest.Config, error) {
	return nil, fmt.Errorf("local operation only")
}
func (noopClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return nil, fmt.Errorf("local operation only")
}
func (noopClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	return nil, fmt.Errorf("local operation only")
}

// NewLocalBuilder returns a builder that is configured not to create REST clients and avoids asking the server for results.
func NewLocalBuilder() *Builder {
	return NewBuilder(noopClientGetter{}).Local()
}

func NewBuilder(restClientGetter RESTClientGetter) *Builder {
	categoryExpanderFn := func() (restmapper.CategoryExpander, error) {
		discoveryClient, err := restClientGetter.ToDiscoveryClient()
		if err != nil {
			return nil, err
		}
		return restmapper.NewDiscoveryCategoryExpander(discoveryClient), err
	}

	return newBuilder(
		restClientGetter.ToRESTConfig,
		restClientGetter.ToRESTMapper,
		(&cachingCategoryExpanderFunc{delegate: categoryExpanderFn}).ToCategoryExpander,
	)
}

func (b *Builder) Schema(schema ContentValidator) *Builder {
	b.schema = schema
	return b
}

func (b *Builder) AddError(err error) *Builder {
	if err == nil {
		return b
	}
	b.errs = append(b.errs, err)
	return b
}

// VisitorConcurrency sets the number of concurrent visitors to use when
// visiting lists.
func (b *Builder) VisitorConcurrency(concurrency int) *Builder {
	b.visitorConcurrency = concurrency
	return b
}
/*
	如下代码是Kubernetes（k8s）中的一个函数，用于处理文件名参数。函数接受两个参数：enforceNamespace和filenameOptions。

	接下来，遍历filenameOptions中的文件路径paths。根据路径的不同情况进行处理：
	最后，如果filenameOptions中指定了Kustomize路径（filenameOptions.Kustomize不为空），创建一个KustomizeVisitor对象并将其添加到Builder的paths字段中。
	如果enforceNamespace为true，调用Builder的RequireNamespace方法。
	最后返回Builder。
*/
// FilenameParam groups input in two categories: URLs and files (files, directories, STDIN)
// If enforceNamespace is false, namespaces in the specs will be allowed to
// override the default namespace. If it is true, namespaces that don't match
// will cause an error.
// If ContinueOnError() is set prior to this method, objects on the path that are not
// recognized will be ignored (but logged at V(2)).
func (b *Builder) FilenameParam(enforceNamespace bool, filenameOptions *FilenameOptions) *Builder {
	// 首先对filenameOptions进行校验，如果有错误则将错误信息添加到Builder的errs字段中，并返回Builder。
	if errs := filenameOptions.validate(); len(errs) > 0 {
		b.errs = append(b.errs, errs...)
		return b
	}
	// 是否递归处理
	recursive := filenameOptions.Recursive
	paths := filenameOptions.Filenames
	// 接下来，遍历filenameOptions中的文件路径paths。根据路径的不同情况进行处理：
	for _, s := range paths {
		switch {
		// 如果路径为"-"，表示从标准输入读取对象，调用Builder的Stdin方法。
		case s == "-":
			b.Stdin()
		// 如果路径以"http://"或"https://"开头，表示是一个URL地址，将URL解析为url.URL对象，并调用Builder的URL方法。
		case strings.Index(s, "http://") == 0 || strings.Index(s, "https://") == 0:
			url, err := url.Parse(s)
			if err != nil {
				b.errs = append(b.errs, fmt.Errorf("the URL passed to filename %q is not valid: %v", s, err))
				continue
			}
			b.URL(defaultHttpGetAttempts, url)
		// 其他情况下，调用expandIfFilePattern函数对路径进行模式匹配展开，获取匹配的文件列表matches。
		// 如果不递归展开（recursive为false）且只有一个匹配，将singleItemImplied字段设置为true。然后调用Builder的Path方法。
		default:
			matches, err := expandIfFilePattern(s)
			if err != nil {
				b.errs = append(b.errs, err)
				continue
			}
			if !recursive && len(matches) == 1 {
				b.singleItemImplied = true
			}
			b.Path(recursive, matches...)
		}
	}
	// 最后，如果filenameOptions中指定了Kustomize路径（filenameOptions.Kustomize不为空），创建一个KustomizeVisitor对象并将其添加到Builder的paths字段中。
	if filenameOptions.Kustomize != "" {
		b.paths = append(
			b.paths,
			&KustomizeVisitor{
				mapper:  b.mapper,
				dirPath: filenameOptions.Kustomize,
				schema:  b.schema,
				fSys:    filesys.MakeFsOnDisk(),
			})
	}
	// 如果enforceNamespace为true，调用Builder的RequireNamespace方法。
	if enforceNamespace {
		b.RequireNamespace()
	}

	// 最后返回Builder
	return b
}

// Unstructured updates the builder so that it will request and send unstructured
// objects. Unstructured objects preserve all fields sent by the server in a map format
// based on the object's JSON structure which means no data is lost when the client
// reads and then writes an object. Use this mode in preference to Internal unless you
// are working with Go types directly.
func (b *Builder) Unstructured() *Builder {
	if b.mapper != nil {
		b.errs = append(b.errs, fmt.Errorf("another mapper was already selected, cannot use unstructured types"))
		return b
	}
	b.objectTyper = unstructuredscheme.NewUnstructuredObjectTyper()
	b.mapper = &mapper{
		localFn:      b.isLocal,
		restMapperFn: b.restMapperFn,
		clientFn:     b.getClient,
		decoder:      &metadataValidatingDecoder{unstructured.UnstructuredJSONScheme},
	}

	return b
}

// WithScheme uses the scheme to manage typing, conversion (optional), and decoding.  If decodingVersions
// is empty, then you can end up with internal types.  You have been warned.
func (b *Builder) WithScheme(scheme *runtime.Scheme, decodingVersions ...schema.GroupVersion) *Builder {
	if b.mapper != nil {
		b.errs = append(b.errs, fmt.Errorf("another mapper was already selected, cannot use internal types"))
		return b
	}
	b.objectTyper = scheme
	codecFactory := serializer.NewCodecFactory(scheme)
	negotiatedSerializer := runtime.NegotiatedSerializer(codecFactory)
	// if you specified versions, you're specifying a desire for external types, which you don't want to round-trip through
	// internal types
	if len(decodingVersions) > 0 {
		negotiatedSerializer = codecFactory.WithoutConversion()
	}
	b.negotiatedSerializer = negotiatedSerializer

	b.mapper = &mapper{
		localFn:      b.isLocal,
		restMapperFn: b.restMapperFn,
		clientFn:     b.getClient,
		decoder:      codecFactory.UniversalDecoder(decodingVersions...),
	}

	return b
}

// LocalParam calls Local() if local is true.
func (b *Builder) LocalParam(local bool) *Builder {
	if local {
		b.Local()
	}
	return b
}

// Local will avoid asking the server for results.
func (b *Builder) Local() *Builder {
	b.local = true
	return b
}

func (b *Builder) isLocal() bool {
	return b.local
}

// Mapper returns a copy of the current mapper.
func (b *Builder) Mapper() *mapper {
	mapper := *b.mapper
	return &mapper
}

// URL accepts a number of URLs directly.
func (b *Builder) URL(httpAttemptCount int, urls ...*url.URL) *Builder {
	for _, u := range urls {
		b.paths = append(b.paths, &URLVisitor{
			URL:              u,
			StreamVisitor:    NewStreamVisitor(nil, b.mapper, u.String(), b.schema),
			HttpAttemptCount: httpAttemptCount,
		})
	}
	return b
}

// Stdin will read objects from the standard input. If ContinueOnError() is set
// prior to this method being called, objects in the stream that are unrecognized
// will be ignored (but logged at V(2)). If StdinInUse() is set prior to this method
// being called, an error will be recorded as there are multiple entities trying to use
// the single standard input stream.
func (b *Builder) Stdin() *Builder {
	b.stream = true
	if b.stdinInUse {
		b.errs = append(b.errs, StdinMultiUseError)
	}
	b.stdinInUse = true
	b.paths = append(b.paths, FileVisitorForSTDIN(b.mapper, b.schema))
	return b
}

// StdinInUse will mark standard input as in use by this Builder, and therefore standard
// input should not be used by another entity. If Stdin() is set prior to this method
// being called, an error will be recorded as there are multiple entities trying to use
// the single standard input stream.
func (b *Builder) StdinInUse() *Builder {
	if b.stdinInUse {
		b.errs = append(b.errs, StdinMultiUseError)
	}
	b.stdinInUse = true
	return b
}

// Stream will read objects from the provided reader, and if an error occurs will
// include the name string in the error message. If ContinueOnError() is set
// prior to this method being called, objects in the stream that are unrecognized
// will be ignored (but logged at V(2)).
func (b *Builder) Stream(r io.Reader, name string) *Builder {
	b.stream = true
	b.paths = append(b.paths, NewStreamVisitor(r, b.mapper, name, b.schema))
	return b
}
/*
	如下代码定义了Builder结构体的Path方法。该方法接受一组路径作为参数，这些路径可以是文件或目录（都可以包含一个或多个资源）。
	对于每个文件，创建一个FileVisitor，然后将每个FileVisitor的内容流式传输给StreamVisitor。如果在调用此方法之前设置了ContinueOnError()，则路径上的无法识别的对象将被忽略（但在V(2)级别记录日志）。
	总的来说，该方法的作用是处理传入的路径参数，将每个路径解析为对应的FileVisitor，并将其添加到Builder的路径列表中。在解析过程中，会检查路径是否存在和是否有错误，并根据情况设置相关字段。
*/
// Path accepts a set of paths that may be files, directories (all can containing
// one or more resources). Creates a FileVisitor for each file and then each
// FileVisitor is streaming the content to a StreamVisitor. If ContinueOnError() is set
// prior to this method being called, objects on the path that are unrecognized will be
// ignored (but logged at V(2)).
func (b *Builder) Path(recursive bool, paths ...string) *Builder {
	// 遍历传入的路径列表。
	for _, p := range paths {
		// 使用os.Stat检查路径是否存在。
		_, err := os.Stat(p)
		if os.IsNotExist(err) {
			// 如果路径不存在，则将错误信息添加到errs字段中，并继续下一个路径的处理。
			b.errs = append(b.errs, fmt.Errorf(pathNotExistError, p))
			continue
		}
		// 如果出现其他错误，则将错误信息添加到errs字段中，并继续下一个路径的处理。
		if err != nil {
			b.errs = append(b.errs, fmt.Errorf("the path %q cannot be accessed: %v", p, err))
			continue
		}
		// 调用ExpandPathsToFileVisitors函数将路径扩展为FileVisitor列表。该函数会根据文件扩展名、递归标志等参数解析路径，并返回对应的FileVisitor列表。
		visitors, err := ExpandPathsToFileVisitors(b.mapper, p, recursive, FileExtensions, b.schema)
		// 如果出现错误，则将错误信息添加到errs字段中。
		if err != nil {
			b.errs = append(b.errs, fmt.Errorf("error reading %q: %v", p, err))
		}
		//  如果返回的FileVisitor列表长度大于1，则设置dir字段为true，表示存在目录。
		if len(visitors) > 1 {
			b.dir = true
		}
		//  将返回的FileVisitor列表添加到paths字段中。
		b.paths = append(b.paths, visitors...)
	}
	//  如果paths字段为空且errs字段也为空，则表示没有正确读取到任何资源文件，将错误信息添加到errs字段中。
	if len(b.paths) == 0 && len(b.errs) == 0 {
		b.errs = append(b.errs, fmt.Errorf("error reading %v: recognized file extensions are %v", paths, FileExtensions))
	}
	// 返回Builder对象。
	return b
}

// ResourceTypes is a list of types of resources to operate on, when listing objects on
// the server or retrieving objects that match a selector.
func (b *Builder) ResourceTypes(types ...string) *Builder {
	b.resources = append(b.resources, types...)
	return b
}

// ResourceNames accepts a default type and one or more names, and creates tuples of
// resources
func (b *Builder) ResourceNames(resource string, names ...string) *Builder {
	for _, name := range names {
		// See if this input string is of type/name format
		tuple, ok, err := splitResourceTypeName(name)
		if err != nil {
			b.errs = append(b.errs, err)
			return b
		}

		if ok {
			b.resourceTuples = append(b.resourceTuples, tuple)
			continue
		}
		if len(resource) == 0 {
			b.errs = append(b.errs, fmt.Errorf("the argument %q must be RESOURCE/NAME", name))
			continue
		}

		// Use the given default type to create a resource tuple
		b.resourceTuples = append(b.resourceTuples, resourceTuple{Resource: resource, Name: name})
	}
	return b
}

// LabelSelectorParam defines a selector that should be applied to the object types to load.
// This will not affect files loaded from disk or URL. If the parameter is empty it is
// a no-op - to select all resources invoke `b.LabelSelector(labels.Everything.String)`.
func (b *Builder) LabelSelectorParam(s string) *Builder {
	selector := strings.TrimSpace(s)
	if len(selector) == 0 {
		return b
	}
	if b.selectAll {
		b.errs = append(b.errs, fmt.Errorf("found non-empty label selector %q with previously set 'all' parameter. ", s))
		return b
	}
	return b.LabelSelector(selector)
}

// LabelSelector accepts a selector directly and will filter the resulting list by that object.
// Use LabelSelectorParam instead for user input.
func (b *Builder) LabelSelector(selector string) *Builder {
	if len(selector) == 0 {
		return b
	}

	b.labelSelector = &selector
	return b
}

// FieldSelectorParam defines a selector that should be applied to the object types to load.
// This will not affect files loaded from disk or URL. If the parameter is empty it is
// a no-op - to select all resources.
func (b *Builder) FieldSelectorParam(s string) *Builder {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return b
	}
	if b.selectAll {
		b.errs = append(b.errs, fmt.Errorf("found non-empty field selector %q with previously set 'all' parameter. ", s))
		return b
	}
	b.fieldSelector = &s
	return b
}

// NamespaceParam accepts the namespace that these resources should be
// considered under from - used by DefaultNamespace() and RequireNamespace()
func (b *Builder) NamespaceParam(namespace string) *Builder {
	b.namespace = namespace
	return b
}

// DefaultNamespace instructs the builder to set the namespace value for any object found
// to NamespaceParam() if empty.
func (b *Builder) DefaultNamespace() *Builder {
	b.defaultNamespace = true
	return b
}

// AllNamespaces instructs the builder to metav1.NamespaceAll as a namespace to request resources
// across all of the namespace. This overrides the namespace set by NamespaceParam().
func (b *Builder) AllNamespaces(allNamespace bool) *Builder {
	if allNamespace {
		b.namespace = metav1.NamespaceAll
	}
	b.allNamespace = allNamespace
	return b
}

// RequireNamespace instructs the builder to set the namespace value for any object found
// to NamespaceParam() if empty, and if the value on the resource does not match
// NamespaceParam() an error will be returned.
func (b *Builder) RequireNamespace() *Builder {
	b.requireNamespace = true
	return b
}

// RequestChunksOf attempts to load responses from the server in batches of size limit
// to avoid long delays loading and transferring very large lists. If unset defaults to
// no chunking.
func (b *Builder) RequestChunksOf(chunkSize int64) *Builder {
	b.limitChunks = chunkSize
	return b
}

// TransformRequests alters API calls made by clients requested from this builder. Pass
// an empty list to clear modifiers.
func (b *Builder) TransformRequests(opts ...RequestTransform) *Builder {
	b.requestTransforms = opts
	return b
}

// Subresource instructs the builder to retrieve the object at the
// subresource path instead of the main resource path.
func (b *Builder) Subresource(subresource string) *Builder {
	b.subresource = subresource
	return b
}

// SelectEverythingParam
func (b *Builder) SelectAllParam(selectAll bool) *Builder {
	if selectAll && (b.labelSelector != nil || b.fieldSelector != nil) {
		b.errs = append(b.errs, fmt.Errorf("setting 'all' parameter but found a non empty selector. "))
		return b
	}
	b.selectAll = selectAll
	return b
}

// ResourceTypeOrNameArgs indicates that the builder should accept arguments
// of the form `(<type1>[,<type2>,...]|<type> <name1>[,<name2>,...])`. When one argument is
// received, the types provided will be retrieved from the server (and be comma delimited).
// When two or more arguments are received, they must be a single type and resource name(s).
// The allowEmptySelector permits to select all the resources (via Everything func).
func (b *Builder) ResourceTypeOrNameArgs(allowEmptySelector bool, args ...string) *Builder {
	args = normalizeMultipleResourcesArgs(args)
	if ok, err := hasCombinedTypeArgs(args); ok {
		if err != nil {
			b.errs = append(b.errs, err)
			return b
		}
		for _, s := range args {
			tuple, ok, err := splitResourceTypeName(s)
			if err != nil {
				b.errs = append(b.errs, err)
				return b
			}
			if ok {
				b.resourceTuples = append(b.resourceTuples, tuple)
			}
		}
		return b
	}
	if len(args) > 0 {
		// Try replacing aliases only in types
		args[0] = b.ReplaceAliases(args[0])
	}
	switch {
	case len(args) > 2:
		b.names = append(b.names, args[1:]...)
		b.ResourceTypes(SplitResourceArgument(args[0])...)
	case len(args) == 2:
		b.names = append(b.names, args[1])
		b.ResourceTypes(SplitResourceArgument(args[0])...)
	case len(args) == 1:
		b.ResourceTypes(SplitResourceArgument(args[0])...)
		if b.labelSelector == nil && allowEmptySelector {
			selector := labels.Everything().String()
			b.labelSelector = &selector
		}
	case len(args) == 0:
	default:
		b.errs = append(b.errs, fmt.Errorf("arguments must consist of a resource or a resource and name"))
	}
	return b
}

// ReplaceAliases accepts an argument and tries to expand any existing
// aliases found in it
func (b *Builder) ReplaceAliases(input string) string {
	replaced := []string{}
	for _, arg := range strings.Split(input, ",") {
		if b.categoryExpanderFn == nil {
			continue
		}
		categoryExpander, err := b.categoryExpanderFn()
		if err != nil {
			b.AddError(err)
			continue
		}

		if resources, ok := categoryExpander.Expand(arg); ok {
			asStrings := []string{}
			for _, resource := range resources {
				if len(resource.Group) == 0 {
					asStrings = append(asStrings, resource.Resource)
					continue
				}
				asStrings = append(asStrings, resource.Resource+"."+resource.Group)
			}
			arg = strings.Join(asStrings, ",")
		}
		replaced = append(replaced, arg)
	}
	return strings.Join(replaced, ",")
}

func hasCombinedTypeArgs(args []string) (bool, error) {
	hasSlash := 0
	for _, s := range args {
		if strings.Contains(s, "/") {
			hasSlash++
		}
	}
	switch {
	case hasSlash > 0 && hasSlash == len(args):
		return true, nil
	case hasSlash > 0 && hasSlash != len(args):
		baseCmd := "cmd"
		if len(os.Args) > 0 {
			baseCmdSlice := strings.Split(os.Args[0], "/")
			baseCmd = baseCmdSlice[len(baseCmdSlice)-1]
		}
		return true, fmt.Errorf("there is no need to specify a resource type as a separate argument when passing arguments in resource/name form (e.g. '%s get resource/<resource_name>' instead of '%s get resource resource/<resource_name>'", baseCmd, baseCmd)
	default:
		return false, nil
	}
}

// Normalize args convert multiple resources to resource tuples, a,b,c d
// as a transform to a/d b/d c/d
func normalizeMultipleResourcesArgs(args []string) []string {
	if len(args) >= 2 {
		resources := []string{}
		resources = append(resources, SplitResourceArgument(args[0])...)
		if len(resources) > 1 {
			names := []string{}
			names = append(names, args[1:]...)
			newArgs := []string{}
			for _, resource := range resources {
				for _, name := range names {
					newArgs = append(newArgs, strings.Join([]string{resource, name}, "/"))
				}
			}
			return newArgs
		}
	}
	return args
}

// splitResourceTypeName handles type/name resource formats and returns a resource tuple
// (empty or not), whether it successfully found one, and an error
func splitResourceTypeName(s string) (resourceTuple, bool, error) {
	if !strings.Contains(s, "/") {
		return resourceTuple{}, false, nil
	}
	seg := strings.Split(s, "/")
	if len(seg) != 2 {
		return resourceTuple{}, false, fmt.Errorf("arguments in resource/name form may not have more than one slash")
	}
	resource, name := seg[0], seg[1]
	if len(resource) == 0 || len(name) == 0 || len(SplitResourceArgument(resource)) != 1 {
		return resourceTuple{}, false, fmt.Errorf("arguments in resource/name form must have a single resource and name")
	}
	return resourceTuple{Resource: resource, Name: name}, true, nil
}

// Flatten will convert any objects with a field named "Items" that is an array of runtime.Object
// compatible types into individual entries and give them their own items. The original object
// is not passed to any visitors.
func (b *Builder) Flatten() *Builder {
	b.flatten = true
	return b
}

// Latest will fetch the latest copy of any objects loaded from URLs or files from the server.
func (b *Builder) Latest() *Builder {
	b.latest = true
	return b
}

// RequireObject ensures that resulting infos have an object set. If false, resulting info may not have an object set.
func (b *Builder) RequireObject(require bool) *Builder {
	b.requireObject = require
	return b
}

// ContinueOnError will attempt to load and visit as many objects as possible, even if some visits
// return errors or some objects cannot be loaded. The default behavior is to terminate after
// the first error is returned from a VisitorFunc.
func (b *Builder) ContinueOnError() *Builder {
	b.continueOnError = true
	return b
}

// SingleResourceType will cause the builder to error if the user specifies more than a single type
// of resource.
func (b *Builder) SingleResourceType() *Builder {
	b.singleResourceType = true
	return b
}

// mappingFor returns the RESTMapping for the Kind given, or the Kind referenced by the resource.
// Prefers a fully specified GroupVersionResource match. If one is not found, we match on a fully
// specified GroupVersionKind, or fallback to a match on GroupKind.
func (b *Builder) mappingFor(resourceOrKindArg string) (*meta.RESTMapping, error) {
	fullySpecifiedGVR, groupResource := schema.ParseResourceArg(resourceOrKindArg)
	gvk := schema.GroupVersionKind{}
	restMapper, err := b.restMapperFn()
	if err != nil {
		return nil, err
	}

	if fullySpecifiedGVR != nil {
		gvk, _ = restMapper.KindFor(*fullySpecifiedGVR)
	}
	if gvk.Empty() {
		gvk, _ = restMapper.KindFor(groupResource.WithVersion(""))
	}
	if !gvk.Empty() {
		return restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	}

	fullySpecifiedGVK, groupKind := schema.ParseKindArg(resourceOrKindArg)
	if fullySpecifiedGVK == nil {
		gvk := groupKind.WithVersion("")
		fullySpecifiedGVK = &gvk
	}

	if !fullySpecifiedGVK.Empty() {
		if mapping, err := restMapper.RESTMapping(fullySpecifiedGVK.GroupKind(), fullySpecifiedGVK.Version); err == nil {
			return mapping, nil
		}
	}

	mapping, err := restMapper.RESTMapping(groupKind, gvk.Version)
	if err != nil {
		// if we error out here, it is because we could not match a resource or a kind
		// for the given argument. To maintain consistency with previous behavior,
		// announce that a resource type could not be found.
		// if the error is _not_ a *meta.NoKindMatchError, then we had trouble doing discovery,
		// so we should return the original error since it may help a user diagnose what is actually wrong
		if meta.IsNoMatchError(err) {
			return nil, fmt.Errorf("the server doesn't have a resource type %q", groupResource.Resource)
		}
		return nil, err
	}

	return mapping, nil
}

func (b *Builder) resourceMappings() ([]*meta.RESTMapping, error) {
	if len(b.resources) > 1 && b.singleResourceType {
		return nil, fmt.Errorf("you may only specify a single resource type")
	}
	mappings := []*meta.RESTMapping{}
	seen := map[schema.GroupVersionKind]bool{}
	for _, r := range b.resources {
		mapping, err := b.mappingFor(r)
		if err != nil {
			return nil, err
		}
		// This ensures the mappings for resources(shortcuts, plural) unique
		if seen[mapping.GroupVersionKind] {
			continue
		}
		seen[mapping.GroupVersionKind] = true

		mappings = append(mappings, mapping)
	}
	return mappings, nil
}

func (b *Builder) resourceTupleMappings() (map[string]*meta.RESTMapping, error) {
	mappings := make(map[string]*meta.RESTMapping)
	canonical := make(map[schema.GroupVersionResource]struct{})
	for _, r := range b.resourceTuples {
		if _, ok := mappings[r.Resource]; ok {
			continue
		}
		mapping, err := b.mappingFor(r.Resource)
		if err != nil {
			return nil, err
		}

		mappings[r.Resource] = mapping
		canonical[mapping.Resource] = struct{}{}
	}
	if len(canonical) > 1 && b.singleResourceType {
		return nil, fmt.Errorf("you may only specify a single resource type")
	}
	return mappings, nil
}

func (b *Builder) visitorResult() *Result {
	if len(b.errs) > 0 {
		return &Result{err: utilerrors.NewAggregate(b.errs)}
	}

	if b.selectAll {
		selector := labels.Everything().String()
		b.labelSelector = &selector
	}

	// visit items specified by paths
	if len(b.paths) != 0 {
		return b.visitByPaths()
	}

	// visit selectors
	if b.labelSelector != nil || b.fieldSelector != nil {
		return b.visitBySelector()
	}

	// visit items specified by resource and name
	if len(b.resourceTuples) != 0 {
		return b.visitByResource()
	}

	// visit items specified by name
	if len(b.names) != 0 {
		return b.visitByName()
	}

	if len(b.resources) != 0 {
		for _, r := range b.resources {
			_, err := b.mappingFor(r)
			if err != nil {
				return &Result{err: err}
			}
		}
		return &Result{err: fmt.Errorf("resource(s) were provided, but no name was specified")}
	}
	return &Result{err: missingResourceError}
}

func (b *Builder) visitBySelector() *Result {
	result := &Result{
		targetsSingleItems: false,
	}

	if len(b.names) != 0 {
		return result.withError(fmt.Errorf("name cannot be provided when a selector is specified"))
	}
	if len(b.resourceTuples) != 0 {
		return result.withError(fmt.Errorf("selectors and the all flag cannot be used when passing resource/name arguments"))
	}
	if len(b.resources) == 0 {
		return result.withError(fmt.Errorf("at least one resource must be specified to use a selector"))
	}
	if len(b.subresource) != 0 {
		return result.withError(fmt.Errorf("subresource cannot be used when bulk resources are specified"))
	}

	mappings, err := b.resourceMappings()
	if err != nil {
		result.err = err
		return result
	}

	var labelSelector, fieldSelector string
	if b.labelSelector != nil {
		labelSelector = *b.labelSelector
	}
	if b.fieldSelector != nil {
		fieldSelector = *b.fieldSelector
	}

	visitors := []Visitor{}
	for _, mapping := range mappings {
		client, err := b.getClient(mapping.GroupVersionKind.GroupVersion())
		if err != nil {
			result.err = err
			return result
		}
		selectorNamespace := b.namespace
		if mapping.Scope.Name() != meta.RESTScopeNameNamespace {
			selectorNamespace = ""
		}
		visitors = append(visitors, NewSelector(client, mapping, selectorNamespace, labelSelector, fieldSelector, b.limitChunks))
	}
	if b.continueOnError {
		result.visitor = EagerVisitorList(visitors)
	} else {
		result.visitor = VisitorList(visitors)
	}
	result.sources = visitors
	return result
}

func (b *Builder) getClient(gv schema.GroupVersion) (RESTClient, error) {
	var (
		client RESTClient
		err    error
	)

	switch {
	case b.fakeClientFn != nil:
		client, err = b.fakeClientFn(gv)
	case b.negotiatedSerializer != nil:
		client, err = b.clientConfigFn.withStdinUnavailable(b.stdinInUse).clientForGroupVersion(gv, b.negotiatedSerializer)
	default:
		client, err = b.clientConfigFn.withStdinUnavailable(b.stdinInUse).unstructuredClientForGroupVersion(gv)
	}

	if err != nil {
		return nil, err
	}

	return NewClientWithOptions(client, b.requestTransforms...), nil
}

func (b *Builder) visitByResource() *Result {
	// if b.singleItemImplied is false, this could be by default, so double-check length
	// of resourceTuples to determine if in fact it is singleItemImplied or not
	isSingleItemImplied := b.singleItemImplied
	if !isSingleItemImplied {
		isSingleItemImplied = len(b.resourceTuples) == 1
	}

	result := &Result{
		singleItemImplied:  isSingleItemImplied,
		targetsSingleItems: true,
	}

	if len(b.resources) != 0 {
		return result.withError(fmt.Errorf("you may not specify individual resources and bulk resources in the same call"))
	}

	// retrieve one client for each resource
	mappings, err := b.resourceTupleMappings()
	if err != nil {
		result.err = err
		return result
	}
	clients := make(map[string]RESTClient)
	for _, mapping := range mappings {
		s := fmt.Sprintf("%s/%s", mapping.GroupVersionKind.GroupVersion().String(), mapping.Resource.Resource)
		if _, ok := clients[s]; ok {
			continue
		}
		client, err := b.getClient(mapping.GroupVersionKind.GroupVersion())
		if err != nil {
			result.err = err
			return result
		}
		clients[s] = client
	}

	items := []Visitor{}
	for _, tuple := range b.resourceTuples {
		mapping, ok := mappings[tuple.Resource]
		if !ok {
			return result.withError(fmt.Errorf("resource %q is not recognized: %v", tuple.Resource, mappings))
		}
		s := fmt.Sprintf("%s/%s", mapping.GroupVersionKind.GroupVersion().String(), mapping.Resource.Resource)
		client, ok := clients[s]
		if !ok {
			return result.withError(fmt.Errorf("could not find a client for resource %q", tuple.Resource))
		}

		selectorNamespace := b.namespace
		if mapping.Scope.Name() != meta.RESTScopeNameNamespace {
			selectorNamespace = ""
		} else {
			if len(b.namespace) == 0 {
				errMsg := "namespace may not be empty when retrieving a resource by name"
				if b.allNamespace {
					errMsg = "a resource cannot be retrieved by name across all namespaces"
				}
				return result.withError(fmt.Errorf(errMsg))
			}
		}

		info := &Info{
			Client:      client,
			Mapping:     mapping,
			Namespace:   selectorNamespace,
			Name:        tuple.Name,
			Subresource: b.subresource,
		}
		items = append(items, info)
	}

	var visitors Visitor
	if b.continueOnError {
		visitors = EagerVisitorList(items)
	} else {
		visitors = VisitorList(items)
	}
	result.visitor = visitors
	result.sources = items
	return result
}

func (b *Builder) visitByName() *Result {
	result := &Result{
		singleItemImplied:  len(b.names) == 1,
		targetsSingleItems: true,
	}

	if len(b.paths) != 0 {
		return result.withError(fmt.Errorf("when paths, URLs, or stdin is provided as input, you may not specify a resource by arguments as well"))
	}
	if len(b.resources) == 0 {
		return result.withError(fmt.Errorf("you must provide a resource and a resource name together"))
	}
	if len(b.resources) > 1 {
		return result.withError(fmt.Errorf("you must specify only one resource"))
	}

	mappings, err := b.resourceMappings()
	if err != nil {
		result.err = err
		return result
	}
	mapping := mappings[0]

	client, err := b.getClient(mapping.GroupVersionKind.GroupVersion())
	if err != nil {
		result.err = err
		return result
	}

	selectorNamespace := b.namespace
	if mapping.Scope.Name() != meta.RESTScopeNameNamespace {
		selectorNamespace = ""
	} else {
		if len(b.namespace) == 0 {
			errMsg := "namespace may not be empty when retrieving a resource by name"
			if b.allNamespace {
				errMsg = "a resource cannot be retrieved by name across all namespaces"
			}
			return result.withError(fmt.Errorf(errMsg))
		}
	}

	visitors := []Visitor{}
	for _, name := range b.names {
		info := &Info{
			Client:      client,
			Mapping:     mapping,
			Namespace:   selectorNamespace,
			Name:        name,
			Subresource: b.subresource,
		}
		visitors = append(visitors, info)
	}
	result.visitor = VisitorList(visitors)
	result.sources = visitors
	return result
}

func (b *Builder) visitByPaths() *Result {
	result := &Result{
		singleItemImplied:  !b.dir && !b.stream && len(b.paths) == 1,
		targetsSingleItems: true,
	}

	if len(b.resources) != 0 {
		return result.withError(fmt.Errorf("when paths, URLs, or stdin is provided as input, you may not specify resource arguments as well"))
	}
	if len(b.names) != 0 {
		return result.withError(fmt.Errorf("name cannot be provided when a path is specified"))
	}
	if len(b.resourceTuples) != 0 {
		return result.withError(fmt.Errorf("resource/name arguments cannot be provided when a path is specified"))
	}

	var visitors Visitor
	if b.continueOnError {
		visitors = EagerVisitorList(b.paths)
	} else {
		visitors = ConcurrentVisitorList{
			visitors:    b.paths,
			concurrency: b.visitorConcurrency,
		}
	}

	if b.flatten {
		visitors = NewFlattenListVisitor(visitors, b.objectTyper, b.mapper)
	}

	// only items from disk can be refetched
	if b.latest {
		// must set namespace prior to fetching
		if b.defaultNamespace {
			visitors = NewDecoratedVisitor(visitors, SetNamespace(b.namespace))
		}
		visitors = NewDecoratedVisitor(visitors, RetrieveLatest)
	}
	if b.labelSelector != nil {
		selector, err := labels.Parse(*b.labelSelector)
		if err != nil {
			return result.withError(fmt.Errorf("the provided selector %q is not valid: %v", *b.labelSelector, err))
		}
		visitors = NewFilteredVisitor(visitors, FilterByLabelSelector(selector))
	}
	result.visitor = visitors
	result.sources = b.paths
	return result
}
/*
	这段代码定义了Builder结构体的Do方法。该方法返回一个Result对象，其中包含由Builder标识的资源的Visitor。该Visitor将遵循ContinueOnError指定的错误行为。注意，流输入会在第一次执行时被消耗 - 可以使用Result的Infos()或Object()方法来捕获进一步迭代的列表。

具体的逻辑如下：

1. 调用b.visitorResult()方法创建一个Result对象r，并将r的mapper字段设置为b.Mapper()返回的mapper对象。
2. 如果r存在错误，直接返回r。
3. 如果b.flatten为true，则将r的visitor字段设置为NewFlattenListVisitor(r.visitor, b.objectTyper, b.mapper)返回的Visitor对象。该Visitor用于将多个资源列表展平为单个资源列表。
4. 创建一个helpers切片，用于存储VisitorFunc函数。
5. 如果b.defaultNamespace为true，则将SetNamespace(b.namespace)函数添加到helpers切片中。该函数用于设置资源的默认命名空间为b.namespace。
6. 如果b.requireNamespace为true，则将RequireNamespace(b.namespace)函数添加到helpers切片中。该函数用于要求资源具有b.namespace命名空间。
7. 将FilterNamespace函数添加到helpers切片中。该函数用于过滤资源，只保留具有正确命名空间的资源。
8. 如果b.requireObject为true，则将RetrieveLazy函数添加到helpers切片中。该函数用于检索懒加载的资源。
9. 如果b.continueOnError为true，则将ContinueOnErrorVisitor{Visitor: r.visitor}设置为r的visitor字段。该Visitor用于在遇到错误时继续执行。
10. 将NewDecoratedVisitor(r.visitor, helpers...)返回的Visitor对象设置为r的visitor字段。该Visitor用于应用一系列的VisitorFunc函数到资源上。
11. 返回Result对象r。

总的来说，Do方法的作用是根据Builder标识的资源创建一个Visitor，并应用一系列的VisitorFunc函数对资源进行处理。最后返回Result对象，其中包含了处理后的Visitor和其他相关信息。
*/
// Do returns a Result object with a Visitor for the resources identified by the Builder.
// The visitor will respect the error behavior specified by ContinueOnError. Note that stream
// inputs are consumed by the first execution - use Infos() or Object() on the Result to capture a list
// for further iteration.
func (b *Builder) Do() *Result {
	r := b.visitorResult()
	r.mapper = b.Mapper()
	if r.err != nil {
		return r
	}
	if b.flatten {
		r.visitor = NewFlattenListVisitor(r.visitor, b.objectTyper, b.mapper)
	}
	helpers := []VisitorFunc{}
	if b.defaultNamespace {
		helpers = append(helpers, SetNamespace(b.namespace))
	}
	if b.requireNamespace {
		helpers = append(helpers, RequireNamespace(b.namespace))
	}
	helpers = append(helpers, FilterNamespace)
	if b.requireObject {
		helpers = append(helpers, RetrieveLazy)
	}
	if b.continueOnError {
		r.visitor = ContinueOnErrorVisitor{Visitor: r.visitor}
	}
	r.visitor = NewDecoratedVisitor(r.visitor, helpers...)
	return r
}

// SplitResourceArgument splits the argument with commas and returns unique
// strings in the original order.
func SplitResourceArgument(arg string) []string {
	out := []string{}
	set := sets.NewString()
	for _, s := range strings.Split(arg, ",") {
		if set.Has(s) {
			continue
		}
		set.Insert(s)
		out = append(out, s)
	}
	return out
}

// HasNames returns true if the provided args contain resource names
func HasNames(args []string) (bool, error) {
	args = normalizeMultipleResourcesArgs(args)
	hasCombinedTypes, err := hasCombinedTypeArgs(args)
	if err != nil {
		return false, err
	}
	return hasCombinedTypes || len(args) > 1, nil
}

// expandIfFilePattern returns all the filenames that match the input pattern
// or the filename if it is a specific filename and not a pattern.
// If the input is a pattern and it yields no result it will result in an error.
func expandIfFilePattern(pattern string) ([]string, error) {
	if _, err := os.Stat(pattern); os.IsNotExist(err) {
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) == 0 {
			return nil, fmt.Errorf(pathNotExistError, pattern)
		}
		if err == filepath.ErrBadPattern {
			return nil, fmt.Errorf("pattern %q is not valid: %v", pattern, err)
		}
		return matches, err
	}
	return []string{pattern}, nil
}

type cachingCategoryExpanderFunc struct {
	delegate CategoryExpanderFunc

	lock   sync.Mutex
	cached restmapper.CategoryExpander
}

func (c *cachingCategoryExpanderFunc) ToCategoryExpander() (restmapper.CategoryExpander, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.cached != nil {
		return c.cached, nil
	}

	ret, err := c.delegate()
	if err != nil {
		return nil, err
	}
	c.cached = ret
	return c.cached, nil
}
