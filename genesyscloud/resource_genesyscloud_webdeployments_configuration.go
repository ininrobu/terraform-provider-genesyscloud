package genesyscloud

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/mypurecloud/platform-client-sdk-go/v56/platformclientv2"
)

var (
	messengerStyle = &schema.Resource{
		Schema: map[string]*schema.Schema{
			"primary_color": {
				Description: "The primary color of messenger in hexadecimal",
				Type:        schema.TypeString,
				Optional:    true,
			},
		},
	}

	launcherButtonSettings = &schema.Resource{
		Schema: map[string]*schema.Schema{
			"visibility": {
				Description: "The visibility settings for the button.Valid values: On, Off, OnDemand",
				Type:        schema.TypeString,
				Optional:    true,
				// RBUTODO : Add validation
			},
		},
	}

	fileUploadMode = &schema.Resource{
		Schema: map[string]*schema.Schema{
			"file_types": {
				Description: "A list of supported content types for uploading files.Valid values: image/jpeg, image/gif, image/png",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				// RBUTODO : Add validation
			},
			"max_file_size_kb": {
				Description: "The maximum file size for file uploads in kilobytes. Default is 10240 (10 MB)",
				Type:        schema.TypeInt,
				Optional:    true,
				// RBUTODO : Add validation 0-10240 and default 10240
			},
		},
	}

	fileUploadSettings = &schema.Resource{
		Schema: map[string]*schema.Schema{
			"modes": {
				Description: "The list of supported file upload modes",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        fileUploadMode,
			},
		},
	}

	messengerSettings = &schema.Resource{
		Schema: map[string]*schema.Schema{
			"enabled": {
				Description: "Whether or not messenger is enabled",
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
			},
			"styles": {
				Description: "The style settings for messenger",
				Type:        schema.TypeList,
				MaxItems:    1,
				Optional:    true,
				Elem:        messengerStyle,
			},
			"launcher_button": {
				Description: "The settings for the launcher button",
				Type:        schema.TypeList,
				MaxItems:    1,
				Optional:    true,
				Elem:        launcherButtonSettings,
			},
			"file_upload": {
				Description: "File upload settings for messenger",
				Type:        schema.TypeList,
				MaxItems:    1,
				Optional:    true,
				Elem:        fileUploadSettings,
			},
		},
	}

	selectorEventTrigger = &schema.Resource{
		Schema: map[string]*schema.Schema{
			"selector": {
				Description: "Element that triggers event",
				Type:        schema.TypeString,
				Required:    true,
			},
			"event_name": {
				Description: "Name of event triggered when element matching selector is interacted with",
				Type:        schema.TypeString,
				Required:    true,
			},
		},
	}

	formsTrackTrigger = &schema.Resource{
		Schema: map[string]*schema.Schema{
			"selector": {
				Description: "Form element that triggers the form submitted or abandoned event",
				Type:        schema.TypeString,
				Required:    true,
			},
			"form_name": {
				Description: "Prefix for the form submitted or abandoned event name",
				Type:        schema.TypeString,
				Required:    true,
			},
			"capture_data_on_form_abandon": {
				Description: "Whether to capture the form data in the form abandoned event",
				Type:        schema.TypeBool,
				Required:    true,
			},
			"capture_data_on_form_submit": {
				Description: "Whether to capture the form data in the form submitted event",
				Type:        schema.TypeBool,
				Required:    true,
			},
		},
	}

	idleEventTrigger = &schema.Resource{
		Schema: map[string]*schema.Schema{
			"event_name": {
				Description: "Name of event triggered after period of inactivity",
				Type:        schema.TypeString,
				Required:    true,
			},
			"idle_after_seconds": {
				Description: "Number of seconds of inactivity before an event is triggered",
				Type:        schema.TypeInt,
				Optional:    true,
			},
		},
	}

	scrollPercentageEventTrigger = &schema.Resource{
		Schema: map[string]*schema.Schema{
			"event_name": {
				Description: "Name of event triggered after scrolling to the specified percentage",
				Type:        schema.TypeString,
				Required:    true,
			},
			"percentage": {
				Description: "Percentage of a webpage at which an event is triggered",
				Type:        schema.TypeInt,
				Required:    true,
				// RBUTODO : Add validation
			},
		},
	}

	journeyEventsSettings = &schema.Resource{
		Schema: map[string]*schema.Schema{
			"enabled": {
				Description: "Whether or not journey event collection is enabled",
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
			},
			"excluded_query_parameters": {
				Description: "List of parameters to be excluded from the query string",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"should_keep_url_fragment": {
				Description: "Whether or not to keep the URL fragment",
				Type:        schema.TypeBool,
				Optional:    true,
			},
			"search_query_parameters": {
				Description: "List of query parameters used for search (e.g. 'q')",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"pageview_config": {
				Description: "Controls how the pageview events are tracked.Valid values: Auto, Once, Off",
				Type:        schema.TypeString,
				Optional:    true,
				// RBUTODO : Add validation
			},
			"click_events": {
				Description: "Details about a selector event trigger",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        selectorEventTrigger,
			},
			"form_track_events": {
				Description: "Details about a forms tracking event trigger",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        formsTrackTrigger,
			},
			"idle_events": {
				Description: "Details about an idle event trigger",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        idleEventTrigger,
			},
			"in_viewport_events": {
				Description: "Details about a selector event trigger",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        selectorEventTrigger,
			},
			"scroll_depth_events": {
				Description: "Details about a scroll percentage event trigger",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        scrollPercentageEventTrigger,
			},
		},
	}
)

func getAllWebDeploymentConfigurations(ctx context.Context, clientConfig *platformclientv2.Configuration) (ResourceIDMetaMap, diag.Diagnostics) {
	resources := make(ResourceIDMetaMap)
	webDeploymentsAPI := platformclientv2.NewWebDeploymentsApiWithConfig(clientConfig)

	configurations, _, getErr := webDeploymentsAPI.GetWebdeploymentsConfigurations(true)
	if getErr != nil {
		return nil, diag.Errorf("Failed to get web deployment configurations: %v", getErr)
	}

	for _, configuration := range *configurations.Entities {
		resources[*configuration.Id] = &ResourceMeta{Name: *configuration.Name}
	}

	return resources, nil
}

func resourceWebDeploymentConfiguration() *schema.Resource {
	return &schema.Resource{
		Description: "Genesys Cloud Web Deployment Configuration",

		CreateContext: createWithPooledClient(createWebDeploymentConfiguration),
		ReadContext:   readWithPooledClient(readWebDeploymentConfiguration),
		UpdateContext: updateWithPooledClient(updateWebDeploymentConfiguration),
		DeleteContext: deleteWithPooledClient(deleteWebDeploymentConfiguration),
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		SchemaVersion: 1,
		Schema: map[string]*schema.Schema{
			"name": {
				Description: "Deployment name",
				Type:        schema.TypeString,
				Required:    true,
				// RBUTODO : Restrict to 100 characters
			},
			"description": {
				Description: "Deployment description",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"languages": {
				Description: "A list of languages supported on the configuration.",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"default_language": {
				Description: "The default language to use for the configuration.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"status": {
				Description: "The current status of the deployment. Valid values: Pending, Active, Inactive, Error, Deleting.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"version": {
				Description: "The version of the configuration.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"messenger": {
				Description: "Settings concerning messenger",
				Type:        schema.TypeList,
				MaxItems:    1,
				Optional:    true,
				Elem:        messengerSettings,
			},
			"journey_events": {
				Description: "Settings concerning journey events",
				Type:        schema.TypeList,
				MaxItems:    1,
				Optional:    true,
				Elem:        journeyEventsSettings,
			},
		},
		CustomizeDiff: customizeConfigurationDiff,
	}
}

func customizeConfigurationDiff(ctx context.Context, diff *schema.ResourceDiff, meta interface{}) error {
	if len(diff.GetChangedKeysPrefix("")) > 0 {
		diff.SetNewComputed("version")
	}
	return nil
}

func waitForConfigurationDraftToBeActive(ctx context.Context, api *platformclientv2.WebDeploymentsApi, id string) diag.Diagnostics {
	return withRetries(ctx, 30*time.Second, func() *resource.RetryError {
		configuration, _, err := api.GetWebdeploymentsConfigurationVersionsDraft(id)
		if err != nil {
			return resource.NonRetryableError(fmt.Errorf("Error verifying active status for new web deployment configuration %s: %s", id, err))
		}

		if *configuration.Status == "Active" {
			return nil
		}

		return resource.RetryableError(fmt.Errorf("Web deployment configuration %s not active yet. Status: %s", id, *configuration.Status))
	})
}

func readWebDeploymentConfigurationFromResourceData(d *schema.ResourceData) (string, *platformclientv2.Webdeploymentconfigurationversion) {
	name := d.Get("name").(string)
	languages := interfaceListToStrings(d.Get("languages").([]interface{}))
	defaultLanguage := d.Get("default_language").(string)

	inputCfg := &platformclientv2.Webdeploymentconfigurationversion{
		Name:            &name,
		Languages:       &languages,
		DefaultLanguage: &defaultLanguage,
	}

	description, ok := d.Get("description").(string)
	if ok {
		inputCfg.Description = &description
	}

	messengerSettings := readMessengerSettings(d)
	if messengerSettings != nil {
		inputCfg.Messenger = messengerSettings
	}

	journeySettings := readJourneySettings(d)
	if journeySettings != nil {
		inputCfg.JourneyEvents = journeySettings
	}

	return name, inputCfg
}

func readJourneySettings(d *schema.ResourceData) *platformclientv2.Journeyeventssettings {
	val, defined := d.GetOk("journey_events")
	if !defined {
		return nil
	}

	cfgs := val.([]interface{})
	if len(cfgs) < 1 {
		return nil
	}

	cfg := cfgs[0].(map[string]interface{})
	enabled, _ := cfg["enabled"].(bool)
	journeySettings := &platformclientv2.Journeyeventssettings{
		Enabled: &enabled,
	}

	return journeySettings
}

func readMessengerSettings(d *schema.ResourceData) *platformclientv2.Messengersettings {
	val, defined := d.GetOk("messenger")
	if !defined {
		return nil
	}

	cfgs := val.([]interface{})
	if len(cfgs) < 1 {
		return nil
	}

	cfg := cfgs[0].(map[string]interface{})
	enabled, _ := cfg["enabled"].(bool)
	messengerSettings := &platformclientv2.Messengersettings{
		Enabled: &enabled,
	}

	styles, ok := cfg["styles"].([]interface{})
	if ok && len(styles) > 0 {
		style := styles[0].(map[string]interface{})
		primaryColor, ok := style["primary_color"].(string)
		if ok {
			messengerSettings.Styles = &platformclientv2.Messengerstyles{
				PrimaryColor: &primaryColor,
			}
		}
	}

	launchers, ok := cfg["launcher_button"].([]interface{})
	if ok && len(launchers) > 0 {
		launcher := launchers[0].(map[string]interface{})
		visibility, ok := launcher["visibility"].(string)
		if ok {
			messengerSettings.LauncherButton = &platformclientv2.Launcherbuttonsettings{
				Visibility: &visibility,
			}
		}
	}

	fileUploads, ok := cfg["file_upload"].([]interface{})
	if ok && len(fileUploads) > 0 {
		fileUpload := fileUploads[0].(map[string]interface{})
		modesCfg, ok := fileUpload["modes"].([]interface{})
		if ok && len(modesCfg) > 0 {
			modes := make([]platformclientv2.Fileuploadmode, len(modesCfg))
			for i, modeCfg := range modesCfg {
				mode, ok := modeCfg.(map[string]interface{})
				if ok {
					maxFileSize := mode["max_file_size_kb"].(int)
					fileTypes := interfaceListToStrings(mode["file_types"].([]interface{}))
					modes[i] = platformclientv2.Fileuploadmode{
						FileTypes:     &fileTypes,
						MaxFileSizeKB: &maxFileSize,
					}
				}
			}

			if len(modes) > 0 {
				messengerSettings.FileUpload = &platformclientv2.Fileuploadsettings{
					Modes: &modes,
				}
			}
		}
	}

	return messengerSettings
}

func createWebDeploymentConfiguration(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	name, inputCfg := readWebDeploymentConfigurationFromResourceData(d)

	log.Printf("Creating web deployment configuration %s", name)

	sdkConfig := meta.(*providerMeta).ClientConfig
	api := platformclientv2.NewWebDeploymentsApiWithConfig(sdkConfig)

	configuration, _, err := api.PostWebdeploymentsConfigurations(*inputCfg)
	if err != nil {
		return diag.Errorf("Failed to create web deployment configuration %s: %s", name, err)
	}

	d.SetId(*configuration.Id)
	d.Set("status", configuration.Status)

	activeError := waitForConfigurationDraftToBeActive(ctx, api, d.Id())
	if activeError != nil {
		return diag.Errorf("Web deployment configuration %s did not become active and could not be published", name)
	}

	configuration, _, err = api.PostWebdeploymentsConfigurationVersionsDraftPublish(d.Id())
	if err != nil {
		return diag.Errorf("Error publishing web deployment configuration %s: %s", name, err)
	}
	d.Set("version", configuration.Version)
	d.Set("status", configuration.Status)

	log.Printf("Created web deployment configuration %s %s", name, *configuration.Id)
	return readWebDeploymentConfiguration(ctx, d, meta)
}

func getVersion(d *schema.ResourceData) string {
	version := d.Get("version").(string)
	if version == "" {
		version = "draft"
	}
	return version
}

func readWebDeploymentConfiguration(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	sdkConfig := meta.(*providerMeta).ClientConfig
	api := platformclientv2.NewWebDeploymentsApiWithConfig(sdkConfig)

	version := getVersion(d)
	log.Printf("Reading web deployment configuration %s", d.Id())
	return withRetriesForRead(ctx, 30*time.Second, d, func() *resource.RetryError {
		configuration, resp, getErr := api.GetWebdeploymentsConfigurationVersion(d.Id(), version)
		if getErr != nil {
			if isStatus404(resp) {
				return resource.RetryableError(fmt.Errorf("Failed to read web deployment configuration %s: %s", d.Id(), getErr))
			}
			return resource.NonRetryableError(fmt.Errorf("Failed to read web deployment configuration %s: %s", d.Id(), getErr))
		}

		d.Set("name", *configuration.Name)
		if configuration.Description != nil {
			d.Set("description", *configuration.Description)
		}
		d.Set("languages", *configuration.Languages)
		d.Set("default_language", *configuration.DefaultLanguage)
		d.Set("status", *configuration.Status)

		log.Printf("Read web deployment configuration %s %s %s", d.Id(), *configuration.Name, d.Get("configuration"))
		return nil
	})
}

func updateWebDeploymentConfiguration(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	name, inputCfg := readWebDeploymentConfigurationFromResourceData(d)

	log.Printf("Updating web deployment configuration %s", name)

	sdkConfig := meta.(*providerMeta).ClientConfig
	api := platformclientv2.NewWebDeploymentsApiWithConfig(sdkConfig)

	_, _, err := api.PutWebdeploymentsConfigurationVersionsDraft(d.Id(), *inputCfg)
	if err != nil {
		return diag.Errorf("Error updating web deployment configuration %s: %s", name, err)
	}

	activeError := waitForConfigurationDraftToBeActive(ctx, api, d.Id())
	if activeError != nil {
		return diag.Errorf("Web deployment configuration %s did not become active and could not be published", name)
	}

	configuration, _, err := api.PostWebdeploymentsConfigurationVersionsDraftPublish(d.Id())
	if err != nil {
		// RBUTODO : Handle the case where there were no changes, which causes a failure with no-changes-from-previous-version?
		return diag.Errorf("Error publishing web deployment configuration %s: %s", name, err)
	}
	d.Set("version", configuration.Version)
	d.Set("status", configuration.Status)

	log.Printf("Finished updating web deployment configuration %s", name)
	time.Sleep(5 * time.Second)
	return readWebDeploymentConfiguration(ctx, d, meta)
}

func deleteWebDeploymentConfiguration(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	name := d.Get("name").(string)

	sdkConfig := meta.(*providerMeta).ClientConfig
	api := platformclientv2.NewWebDeploymentsApiWithConfig(sdkConfig)

	log.Printf("Deleting web deployment configuration %s", name)
	_, err := api.DeleteWebdeploymentsConfiguration(d.Id())

	if err != nil {
		return diag.Errorf("Failed to delete web deployment configuration %s: %s", name, err)
	}

	return withRetries(ctx, 30*time.Second, func() *resource.RetryError {
		_, resp, err := api.GetWebdeploymentsConfigurationVersionsDraft(d.Id())
		if err != nil {
			if isStatus404(resp) {
				log.Printf("Deleted web deployment configuration %s", d.Id())
				return nil
			}
			return resource.NonRetryableError(fmt.Errorf("Error deleting web deployment configuration %s: %s", d.Id(), err))
		}

		return resource.RetryableError(fmt.Errorf("Web deployment configuration %s still exists", d.Id()))
	})
}
