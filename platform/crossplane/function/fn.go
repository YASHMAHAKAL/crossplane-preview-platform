package main

import (
	"context"
	"encoding/json"
	"fmt"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"
)

// RunFunction is built from this exact PR head for the isolated candidate gate.
func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	rsp := response.To(req, response.DefaultTTL)
	input := req.GetInput().AsMap()
	mode, ok := input["mode"].(string)
	if !ok {
		response.Fatal(rsp, fmt.Errorf("composition input mode is required"))
		return rsp, nil
	}
	observed, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("read XR: %w", err))
		return rsp, nil
	}
	raw, err := json.Marshal(observed.Resource.Object)
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("encode XR: %w", err))
		return rsp, nil
	}
	var xr PreviewXR
	if err := json.Unmarshal(raw, &xr); err != nil {
		response.Fatal(rsp, fmt.Errorf("decode XR: %w", err))
		return rsp, nil
	}
	rendered, err := Render(xr, mode)
	if err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}
	observedResources, err := request.GetObservedComposedResources(req)
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("read composed resources: %w", err))
		return rsp, nil
	}
	desired := map[resource.Name]*resource.DesiredComposed{}
	for _, item := range rendered {
		cd := composed.New()
		cd.Object = item.Object
		state := resource.ReadyFalse
		if current, ok := observedResources[resource.Name(item.Name)]; ok && current.Resource != nil && composedReady(item.Name, current.Resource.Object) {
			state = resource.ReadyTrue
		}
		desired[resource.Name(item.Name)] = &resource.DesiredComposed{Resource: cd, Ready: state}
	}
	if err := response.SetDesiredComposedResources(rsp, desired); err != nil {
		response.Fatal(rsp, fmt.Errorf("set desired resources: %w", err))
	}
	return rsp, nil
}
