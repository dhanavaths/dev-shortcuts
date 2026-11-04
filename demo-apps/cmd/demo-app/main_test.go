package main

import "testing"

func TestCountHandlerCallIsOptIn(t *testing.T) {
	const handler = "livenessHandler"
	counter := handlerCallCounters[handler]
	originalSetting := logHandlerCallCounts
	originalCount := counter.Load()
	t.Cleanup(func() {
		logHandlerCallCounts = originalSetting
		counter.Store(originalCount)
	})

	counter.Store(0)
	logHandlerCallCounts = false
	countHandlerCall(handler)
	if got := counter.Load(); got != 0 {
		t.Fatalf("countHandlerCall() count with logging disabled = %d, want 0", got)
	}

	logHandlerCallCounts = true
	countHandlerCall(handler)
	if got := counter.Load(); got != 1 {
		t.Fatalf("countHandlerCall() count with logging enabled = %d, want 1", got)
	}
}

func TestNewGrpcServerRegistersReflection(t *testing.T) {
	server := newGrpcServer()
	services := server.GetServiceInfo()

	for _, service := range []string{
		"grpcdemo.GrpcDemoService",
		"grpc.reflection.v1.ServerReflection",
		"grpc.reflection.v1alpha.ServerReflection",
	} {
		if _, ok := services[service]; !ok {
			t.Errorf("service %q is not registered", service)
		}
	}

	registeredMethods := make(map[string]bool)
	for _, method := range services["grpcdemo.GrpcDemoService"].Methods {
		registeredMethods[method.Name] = true
	}
	for _, method := range []string{"Sync", "Inc"} {
		if !registeredMethods[method] {
			t.Errorf("GrpcDemoService.%s is not registered", method)
		}
	}
}
