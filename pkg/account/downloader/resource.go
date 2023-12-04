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

package downloader

import (
	"context"
	"github.com/dynatrace/dynatrace-configuration-as-code-core/api/clients/accounts"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/account"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/account/downloader/internal/http"
)

type Account struct {
	httpClient  *accounts.Client
	accountInfo *account.AccountInfo
	httpClient2 *http.Client
}

func New(accountInfo *account.AccountInfo, client *accounts.Client) *Account {
	return &Account{
		httpClient:  client,
		accountInfo: accountInfo,
		httpClient2: (*http.Client)(client),
	}
}

func (a *Account) DownloadConfiguration() (*account.Resources, error) {
	ctx := context.TODO()

	tenants, err := a.environments(ctx)
	if err != nil {
		return nil, err
	}

	policies, err := a.policies(ctx)
	if err != nil {
		return nil, err
	}

	groups, err := a.groups(ctx, policies, tenants)
	if err != nil {
		return nil, err
	}

	users, err := a.users(ctx, groups)
	if err != nil {
		return nil, err
	}

	r := account.Resources{
		Users:    users.asAccountUsers(),
		Groups:   groups.asAccountGroups(),
		Policies: policies.asAccountPolicies(),
	}

	return &r, nil
}