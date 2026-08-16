package webchange

import "time"

// _fetchBackendWebDriver defines the fetch backend constant for HTML WebDriver.
const _fetchBackendWebDriver = "html_webdriver"

// _checkIntervalMinutes specifies how often changedetection.io re-checks a
// watched URL.
const _checkIntervalMinutes = 3

// Watch represents a changedetection.io watch configuration.
type Watch struct {
	// URL is the URL to be monitored.
	URL string

	// Unreachable indicates whether the URL is currently unreachable.
	Unreachable bool

	// LastChangedAt holds the timestamp of the last change detected.
	LastChangedAt time.Time
}

// rawWatch represents a changedetection.io watch configuration.
type rawWatch struct {
	// UUID is the unique identifier of the watch.
	UUID string `json:"uuid"`

	// URL is the URL to be monitored.
	URL string `json:"url"`

	// TimeBetweenCheck specifies the interval between checks.
	TimeBetweenCheck timeBetweenCheck `json:"time_between_check"`

	// LastChanged holds the timestamp of the last change detected.
	LastChanged int64 `json:"last_changed"`

	// LastError holds the last error message encountered during checks.
	LastError any `json:"last_error"`
}

// timeBetweenCheck represents the interval between watch checks.
type timeBetweenCheck struct {
	// Minutes represents the number of minutes.
	Minutes int `json:"minutes"`
}

// watchRequest represents the request body for creating or updating a watch.
type watchRequest struct {
	// URL is the URL to be monitored.
	URL string `json:"url"`

	// TimeBetweenCheck specifies the interval between checks.
	TimeBetweenCheck *timeBetweenCheck `json:"time_between_check"`

	// FetchBackend specifies the fetch backend to be used.
	FetchBackend string `json:"fetch_backend"`

	// TimeBetweenCheckUseDefault indicates if the default interval is used.
	TimeBetweenCheckUseDefault bool `json:"time_between_check_use_default"`
}
