// Copyright 2026 Microsoft Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package detach removes Azure deployment stacks while leaving every resource they manage in place.
package detach

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeploymentstacks"
)

// Result describes what AtResourceGroup did.
type Result string

const (
	// ResultDetached means the stack existed and was removed; its resources were detached.
	ResultDetached Result = "Detached"
	// ResultNotFound means the stack or its resource group did not exist, so nothing changed.
	ResultNotFound Result = "NotFound"
)

// deleteOptions always detaches. There is deliberately no way to ask for a delete: removing a stack must never
// remove the resources it manages.
func deleteOptions() *armdeploymentstacks.ClientBeginDeleteAtResourceGroupOptions {
	return &armdeploymentstacks.ClientBeginDeleteAtResourceGroupOptions{
		UnmanageActionResources:        to.Ptr(armdeploymentstacks.UnmanageActionResourceModeDetach),
		UnmanageActionResourceGroups:   to.Ptr(armdeploymentstacks.UnmanageActionResourceGroupModeDetach),
		UnmanageActionManagementGroups: to.Ptr(armdeploymentstacks.UnmanageActionManagementGroupModeDetach),
	}
}

// AtResourceGroup removes the deployment stack name from resourceGroup and detaches every resource, resource
// group and management group it manages. A stack or resource group that does not exist is not an error.
func AtResourceGroup(ctx context.Context, client *armdeploymentstacks.Client, resourceGroup, name string) (Result, error) {
	logger := logr.FromContextOrDiscard(ctx).WithValues("resourceGroup", resourceGroup, "deploymentStack", name)

	stack, err := client.GetAtResourceGroup(ctx, resourceGroup, name, nil)
	if code, ok := notFoundCode(err); ok {
		logger.Info("Deployment stack not found, nothing to detach.", "errorCode", code)
		return ResultNotFound, nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get deployment stack %s in resource group %s: %w", name, resourceGroup, err)
	}

	var managed int
	if stack.Properties != nil {
		managed = len(stack.Properties.Resources)
	}
	logger.Info("Removing deployment stack and detaching its resources.", "managedResources", managed)

	poller, err := client.BeginDeleteAtResourceGroup(ctx, resourceGroup, name, deleteOptions())
	if code, ok := notFoundCode(err); ok {
		logger.Info("Deployment stack disappeared before it could be removed, nothing to detach.", "errorCode", code)
		return ResultNotFound, nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to start removing deployment stack %s in resource group %s: %w", name, resourceGroup, err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return "", fmt.Errorf("failed to remove deployment stack %s in resource group %s: %w", name, resourceGroup, err)
	}

	logger.Info("Deployment stack removed; its resources were detached.", "managedResources", managed)
	return ResultDetached, nil
}

// notFoundCode reports whether err is an ARM 404, which covers both a missing stack and a missing resource group,
// and returns the ARM error code so the log shows which one it was.
func notFoundCode(err error) (string, bool) {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) && respErr.StatusCode == http.StatusNotFound {
		return respErr.ErrorCode, true
	}
	return "", false
}
