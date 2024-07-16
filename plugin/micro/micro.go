package micro

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	pb "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/shockerjue/protoc-gen-micro/generator"
)

// Paths for packages used by code generated in this file,
// relative to the import_prefix of the generator.Generator.
const (
	contextPkgPath = "context"
	clientPkgPath  = "github.com/shockerjue/gffg/client"
	serverPkgPath  = "github.com/shockerjue/gffg/server"
	commonPkgPath  = "github.com/shockerjue/gffg/common"
)

func init() {
	generator.RegisterPlugin(new(micro))
}

// micro is an implementation of the Go protocol buffer compiler's
// plugin architecture.  It generates bindings for go-micro support.
type micro struct {
	gen *generator.Generator
}

// Name returns the name of this plugin, "micro".
func (g *micro) Name() string {
	return "micro"
}

// The names for packages imported in the generated code.
// They may vary from the final path component of the import path
// if the name is used by other packages.
var (
	contextPkg string
	clientPkg  string
	serverPkg  string
	commonPkg  string
)

// Init initializes the plugin.
func (g *micro) Init(gen *generator.Generator) {
	g.gen = gen
	contextPkg = generator.RegisterUniquePackageName("context", nil)
	clientPkg = generator.RegisterUniquePackageName("client", nil)
	serverPkg = generator.RegisterUniquePackageName("server", nil)
	commonPkg = generator.RegisterUniquePackageName("common", nil)
}

// Given a type name defined in a .proto, return its object.
// Also record that we're using it, to guarantee the associated import.
func (g *micro) objectNamed(name string) generator.Object {
	g.gen.RecordTypeUse(name)
	return g.gen.ObjectNamed(name)
}

// Given a type name defined in a .proto, return its name as we will print it.
func (g *micro) typeName(str string) string {
	return g.gen.TypeName(g.objectNamed(str))
}

// P forwards to g.gen.P.
func (g *micro) P(args ...interface{}) { g.gen.P(args...) }

// Generate generates code for the services in the given file.
func (g *micro) Generate(file *generator.FileDescriptor) {
	if len(file.FileDescriptorProto.Service) == 0 {
		return
	}

	for i, service := range file.FileDescriptorProto.Service {
		g.generateService(file, service, i)
	}
}

// GenerateImports generates the import declaration for this file.
func (g *micro) GenerateImports(file *generator.FileDescriptor) {
	if len(file.FileDescriptorProto.Service) == 0 {
		return
	}
	g.P("import (")
	g.P(`"errors"`)
	g.P(clientPkg, " ", strconv.Quote(path.Join(g.gen.ImportPrefix, clientPkgPath)))
	g.P(serverPkg, " ", strconv.Quote(path.Join(g.gen.ImportPrefix, serverPkgPath)))
	g.P(contextPkg, " ", strconv.Quote(path.Join(g.gen.ImportPrefix, contextPkgPath)))
	g.P(commonPkg, " ", strconv.Quote(path.Join(g.gen.ImportPrefix, commonPkgPath)))
	g.P(")")
	g.P()
}

// reservedClientName records whether a client name is reserved on the client side.
var reservedClientName = map[string]bool{
	// TODO: do we need any in go-micro?
}

func unexport(s string) string { return strings.ToLower(s[:1]) + s[1:] }

// generateService generates all the code for the named service.
func (g *micro) generateService(file *generator.FileDescriptor, service *pb.ServiceDescriptorProto, index int) {
	path := fmt.Sprintf("6,%d", index) // 6 means service.

	origServName := service.GetName()
	serviceName := strings.ToLower(service.GetName())
	if pkg := file.GetPackage(); pkg != "" {
		serviceName = pkg
	}
	servName := generator.CamelCase(origServName)

	g.P()
	g.P("// Client API for ", servName, " service")
	g.P()

	// Client interface.
	g.P("type ", servName, " interface {")
	for i, method := range service.Method {
		g.gen.PrintComments(fmt.Sprintf("%s,2,%d", path, i)) // 2 means method in a service.
		g.P(g.generateClientSignature(servName, method))
	}
	g.P("}")
	g.P()

	// Client structure.
	g.P("type ", unexport(servName), " struct {")
	g.P("serviceName string")
	g.P("c *client.Client")
	g.P("}")
	g.P()

	// NewClient factory.
	g.P("func New", servName, " (c *client.Client, serviceName string", "", ") ", servName, " {")
	g.P("if len(serviceName) == 0 {")
	g.P(`serviceName = "`, serviceName, `"`)
	g.P("}")
	g.P("return &", unexport(servName), "{")
	g.P("serviceName: serviceName,")
	g.P("c: c,")
	g.P("}")
	g.P("}")
	g.P()
	var methodIndex, streamIndex int
	serviceDescVar := "_" + servName + "_serviceDesc"
	// Client method implementations.
	for _, method := range service.Method {
		var descExpr string
		if !method.GetServerStreaming() {
			// Unary RPC method
			descExpr = fmt.Sprintf("&%s.Methods[%d]", serviceDescVar, methodIndex)
			methodIndex++
		} else {
			// Streaming RPC method
			descExpr = fmt.Sprintf("&%s.Streams[%d]", serviceDescVar, streamIndex)
			streamIndex++
		}
		g.generateClientMethod(serviceName, servName, serviceDescVar, method, descExpr)
	}

	g.P("// Server API for ", servName, " service")
	g.P()

	// Server interface.
	serverType := servName + "Handler"
	g.P("type ", serverType, " interface {")
	for i, method := range service.Method {
		g.gen.PrintComments(fmt.Sprintf("%s,2,%d", path, i)) // 2 means method in a service.
		g.P(g.generateServerSignature(servName, method))
	}
	g.P("}")
	g.P()

	// Server registration.
	g.P("func Register", servName, "Handler(s *", "server", ".Server, hdlr ", serverType, ", opts ...", serverPkg, ".HandlerOption) {")
	g.P("type ", unexport(origServName), " interface {")
	for _, method := range service.Method {
		g.generateInterfaceSignature(servName, method)
	}
	g.P("}")
	g.P("type ", servName, " struct {")
	g.P(unexport(servName))
	g.P("}")
	g.P("h := &", unexport(servName)+"Handler", "{hdlr}")
	g.P("handler := server.RpcHandler()")
	for _, method := range service.Method {
		methName := generator.CamelCase(method.GetName())
		g.P("handler.Add(common.GenRid(\"", servName, ".", methName, "\"),&server.RpcItem {")
		g.P("Call:h.", methName, ",")
		g.P("Name:\"", servName, ".", methName, "\",")
		g.P("})")
	}
	g.P("s.NewHandler(handler)")
	g.P("}")
	g.P()

	// Handler type
	g.P("type ", unexport(serverType), " struct {")
	g.P(serverType)
	g.P("}")
	g.P()

	// Server handler implementations.
	var handlerNames []string
	for _, method := range service.Method {
		hname := g.generateServerMethod(servName, method)
		handlerNames = append(handlerNames, hname)
	}
}

// generateClientSignature returns the client-side signature for a method.
func (g *micro) generateClientSignature(servName string, method *pb.MethodDescriptorProto) string {
	origMethName := method.GetName()
	methName := generator.CamelCase(origMethName)
	if reservedClientName[methName] {
		methName += "_"
	}
	reqArg := ", in *" + g.typeName(method.GetInputType())
	if method.GetClientStreaming() {
		reqArg = ""
	}
	respName := "out *" + g.typeName(method.GetOutputType())
	if method.GetServerStreaming() || method.GetClientStreaming() {
		respName = servName + "_" + generator.CamelCase(origMethName) + "Client"
	}

	return fmt.Sprintf("%s(ctx %s.Context%s, opts ...client.CallOption) (%s, err error)", methName, contextPkg, reqArg, respName)
}

func (g *micro) generateClientMethod(reqServ, servName, serviceDescVar string, method *pb.MethodDescriptorProto, descExpr string) {
	reqMethod := fmt.Sprintf("%s.%s", servName, method.GetName())
	outType := g.typeName(method.GetOutputType())

	g.P("func (c *", unexport(servName), ") ", g.generateClientSignature(servName, method), "{")
	g.P("if nil == in {")
	g.P(`	err = errors.New("`, reqMethod, ` req is nil")`)
	g.P("   return")
	g.P("}")
	g.P(`req := c.c.NewRequest(c.serviceName, "`, reqMethod, `", in)`)
	g.P("out = new(", outType, ")")
	// TODO: Pass descExpr to Invoke.
	g.P(`res, err := c.c.Call(ctx, req, in, opts...)`)
	g.P("if err != nil { return  }")
	g.P("err = out.Unmarshal(res)")
	g.P("return")
	g.P("}")
	g.P()

	return
}

// generateServerSignature returns the server-side signature for a method.
func (g *micro) generateServerSignature(servName string, method *pb.MethodDescriptorProto) string {
	origMethName := method.GetName()
	methName := generator.CamelCase(origMethName)
	if reservedClientName[methName] {
		methName += "_"
	}

	var reqArgs []string
	reqArgs = append(reqArgs, contextPkg+".Context")
	reqArgs = append(reqArgs, "*"+g.typeName(method.GetInputType()))

	ret := "(*" + g.typeName(method.GetOutputType()) + ", error)"
	return methName + "(" + strings.Join(reqArgs, ", ") + ") " + ret
}

func (g *micro) generateServerInterface(servName string, method *pb.MethodDescriptorProto) string {
	methName := generator.CamelCase(method.GetName())
	hname := fmt.Sprintf("_%s_%s_Handler", servName, methName)
	inType := g.typeName(method.GetInputType())
	outType := g.typeName(method.GetOutputType())

	g.P(methName, "(context.Context", ", *", inType, ") (*", outType, ",error)")
	return hname
}

func (g *micro) generateInterfaceSignature(servName string, method *pb.MethodDescriptorProto) string {
	methName := generator.CamelCase(method.GetName())
	hname := fmt.Sprintf("_%s_%s_Handler", servName, methName)

	g.P(methName, "(ctx context.Context, in []byte) (out []byte, err error)")
	return hname
}

func (g *micro) generateServerMethod(servName string, method *pb.MethodDescriptorProto) string {
	methName := generator.CamelCase(method.GetName())
	hname := fmt.Sprintf("_%s_%s_Handler", servName, methName)
	serveType := servName + "Handler"
	inType := g.typeName(method.GetInputType())
	outType := g.typeName(method.GetOutputType())

	g.P("func (h *", unexport(serveType), ") ", methName, "(ctx context.Context", ", in []byte", ") (out []byte, err error) {")
	g.P("var req ", inType)
	g.P("err = req.Unmarshal(in)")
	g.P("if nil != err { return }")
	g.P()
	g.P("var res *", outType)
	g.P("res, err = h.", serveType, ".", methName, "(ctx, &req)")
	g.P("if nil != err { return }")
	g.P()
	g.P("out, err = res.Marshal()")
	g.P("if nil != err { return }")
	g.P("return")
	g.P("}")
	g.P()
	return hname
}
