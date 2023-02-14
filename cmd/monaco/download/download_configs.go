// @license
// Copyright 2021 Dynatrace LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package download

import (
	"fmt"
	"os"
	"strings"

	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/api"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/client"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/download"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/download/classic"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/download/settings"
	project "github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/project/v2"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util/log"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util/maps"
	"github.com/spf13/afero"
)

type downloadCommandOptions struct {
	downloadCommandOptionsShared
	specificAPIs    []string
	specificSchemas []string
	onlyAPIs        bool
	onlySettings    bool
}

type manifestDownloadOptions struct {
	manifestFile            string
	specificEnvironmentName string
	downloadCommandOptions
}

type directDownloadOptions struct {
	environmentUrl, envVarName string
	downloadCommandOptions
}

func (d DefaultCommand) DownloadConfigsBasedOnManifest(fs afero.Fs, cmdOptions manifestDownloadOptions) error {

	envUrl, token, tokenEnvVar, err := getEnvFromManifest(fs, cmdOptions.manifestFile, cmdOptions.specificEnvironmentName, cmdOptions.projectName)
	if err != nil {
		return err
	}

	if !cmdOptions.forceOverwrite {
		cmdOptions.projectName = fmt.Sprintf("%s_%s", cmdOptions.projectName, cmdOptions.specificEnvironmentName)
	}

	concurrentDownloadLimit := concurrentRequestLimitFromEnv()

	options := downloadOptions{
		downloadOptionsShared: downloadOptionsShared{
			environmentUrl:          envUrl,
			token:                   token,
			tokenEnvVarName:         tokenEnvVar,
			outputFolder:            cmdOptions.outputFolder,
			projectName:             cmdOptions.projectName,
			forceOverwriteManifest:  cmdOptions.forceOverwrite,
			clientProvider:          client.NewDynatraceClient,
			concurrentDownloadLimit: concurrentDownloadLimit,
		},
		specificAPIs:    cmdOptions.specificAPIs,
		specificSchemas: cmdOptions.specificSchemas,
		onlyAPIs:        cmdOptions.onlyAPIs,
		onlySettings:    cmdOptions.onlySettings,
	}
	return doDownloadConfigs(fs, api.NewApis(), options)
}

func (d DefaultCommand) DownloadConfigs(fs afero.Fs, cmdOptions directDownloadOptions) error {
	token := os.Getenv(cmdOptions.envVarName)
	concurrentDownloadLimit := concurrentRequestLimitFromEnv()
	errors := validateParameters(cmdOptions.envVarName, cmdOptions.environmentUrl, cmdOptions.projectName, token)

	if len(errors) > 0 {
		return PrintAndFormatErrors(errors, "not all necessary information is present to start downloading configurations")
	}

	options := downloadOptions{
		downloadOptionsShared: downloadOptionsShared{
			environmentUrl:          cmdOptions.environmentUrl,
			token:                   token,
			tokenEnvVarName:         cmdOptions.envVarName,
			outputFolder:            cmdOptions.outputFolder,
			projectName:             cmdOptions.projectName,
			forceOverwriteManifest:  cmdOptions.forceOverwrite,
			clientProvider:          client.NewDynatraceClient,
			concurrentDownloadLimit: concurrentDownloadLimit,
		},
		specificAPIs:    cmdOptions.specificAPIs,
		specificSchemas: cmdOptions.specificSchemas,
		onlyAPIs:        cmdOptions.onlyAPIs,
		onlySettings:    cmdOptions.onlySettings,
	}
	return doDownloadConfigs(fs, api.NewApis(), options)
}

type downloadOptions struct {
	downloadOptionsShared
	specificAPIs    []string
	specificSchemas []string
	onlyAPIs        bool
	onlySettings    bool
}

func doDownloadConfigs(fs afero.Fs, apis api.ApiMap, opts downloadOptions) error {
	err := preDownloadValidations(fs, opts.downloadOptionsShared)
	if err != nil {
		return err
	}

	log.Info("Downloading from environment '%v' into project '%v'", opts.environmentUrl, opts.projectName)

	downloadedConfigs, err := downloadConfigs(apis, opts)

	if err != nil {
		return err
	}

	log.Info("Resolving dependencies between configurations")
	downloadedConfigs = download.ResolveDependencies(downloadedConfigs)

	return writeConfigs(downloadedConfigs, opts.downloadOptionsShared, err, fs)
}

func downloadConfigs(apis api.ApiMap, opts downloadOptions) (project.ConfigsPerType, error) {
	c, err := opts.getDynatraceClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Dynatrace client: %w", err)
	}

	c = client.LimitClientParallelRequests(c, opts.concurrentDownloadLimit)

	apisToDownload, errors := getApisToDownload(apis, opts.specificAPIs)
	if len(errors) > 0 {
		err = PrintAndFormatErrors(errors, "failed to load apis")
		return nil, err
	}

	configObjects := make(project.ConfigsPerType)

	// download specific APIs only
	if len(opts.specificAPIs) > 0 {
		log.Debug("APIs to download: \n - %v", strings.Join(maps.Keys(apisToDownload), "\n - "))
		c := classic.DownloadAllConfigs(apisToDownload, c, opts.projectName)
		maps.Copy(configObjects, c)
	}

	// download specific settings only
	if len(opts.specificSchemas) > 0 {
		log.Debug("Settings to download: \n - %v", strings.Join(opts.specificSchemas, "\n - "))
		s := settings.Download(c, opts.specificSchemas, opts.projectName)
		maps.Copy(configObjects, s)
	}

	// return specific download objects
	if len(opts.specificSchemas) > 0 || len(opts.specificAPIs) > 0 {
		return configObjects, nil
	}

	// if nothing was specified specifically, lets download all configs and settings
	if !opts.onlySettings {
		log.Debug("APIs to download: \n - %v", strings.Join(maps.Keys(apisToDownload), "\n - "))
		configObjects = classic.DownloadAllConfigs(apisToDownload, c, opts.projectName)
	}
	if !opts.onlyAPIs {
		settingsObjects := settings.DownloadAll(c, opts.projectName)
		maps.Copy(configObjects, settingsObjects)
	}
	return configObjects, nil
}

// Get all v2 apis and filter for the selected ones
func getApisToDownload(apis api.ApiMap, specificAPIs []string) (api.ApiMap, []error) {
	var errors []error

	apisToDownload, unknownApis := apis.FilterApisByName(specificAPIs)
	if len(unknownApis) > 0 {
		errors = append(errors, fmt.Errorf("APIs '%v' are not known. Please consult our documentation for known API-names", strings.Join(unknownApis, ",")))
	}

	if len(specificAPIs) == 0 {
		var deprecated api.ApiMap
		apisToDownload, deprecated = apisToDownload.Filter(deprecatedEndpointFilter)
		for _, d := range deprecated {
			log.Warn("API '%s' is deprecated by '%s' and will not be downloaded", d.GetId(), d.DeprecatedBy())
		}
	}

	apisToDownload, filtered := apisToDownload.Filter(func(api api.Api) bool {
		return api.ShouldSkipDownload()
	})

	if len(filtered) > 0 {
		keys := strings.Join(maps.Keys(filtered), ", ")
		log.Info("APIs that won't be downloaded and need manual creation: '%v'.", keys)
	}

	if len(apisToDownload) == 0 {
		errors = append(errors, fmt.Errorf("no APIs to download"))
	}

	return apisToDownload, errors
}

func deprecatedEndpointFilter(api api.Api) bool {
	return api.DeprecatedBy() != ""
}