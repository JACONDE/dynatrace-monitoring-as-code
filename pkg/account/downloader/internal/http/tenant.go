/*
 * @license
 * Copyright 2023 Dynatrace LLC
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package http

import (
	"context"
	accountmanagement "github.com/dynatrace/dynatrace-configuration-as-code-core/gen/account_management"
)

func (c *Client) GetTenants(ctx context.Context, account string) ([]accountmanagement.EnvironmentDto, error) {
	r, resp, err := c.EnvironmentManagementAPI.GetEnvironments(ctx, account).Execute()
	defer closeResponseBody(resp)

	if err = handleClientResponseError(resp, err); err != nil {
		return nil, err
	}

	return r.Data, nil
}