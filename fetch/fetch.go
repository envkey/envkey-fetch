package fetch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/envkey/envkey-fetch/cache"
	"github.com/envkey/envkey-fetch/parser"
	"github.com/envkey/envkey-fetch/version"
	"github.com/envkey/myhttp"
)

type FetchOptions struct {
	ShouldCache   bool
	CacheDir      string
	ClientName    string
	ClientVersion string
}

var DefaultHost = "env.envkey.com"
var BackupDefaultHost = "s3-eu-west-1.amazonaws.com/envkey-backup/envs"
var ApiVersion = 1
var HttpGetter = myhttp.New(time.Second * 2)

func Fetch(envkey string, options FetchOptions) string {
	if len(strings.Split(envkey, "-")) < 2 {
		return "error: ENVKEY invalid"
	}

	var fetchCache *cache.Cache

	if options.ShouldCache {
		// If initializing cache fails for some reason, ignore and let it be nil
		fetchCache, _ = cache.NewCache(options.CacheDir)
	}

	response, envkeyParam, pw, err := fetchEnv(envkey, options, fetchCache)
	if err != nil {
		return "error: " + err.Error()
	}
	res, err := response.Parse(pw)
	if err != nil {
		if fetchCache != nil {
			fetchCache.Delete(envkeyParam)
		}
		return "error: ENVKEY invalid"
	}

	// Ensure cache bizness finished (don't worry about error)
	if fetchCache != nil {
		select {
		case <-fetchCache.Done:
		default:
		}
	}

	return res
}

func fetchEnv(envkey string, options FetchOptions, fetchCache *cache.Cache) (*parser.EnvServiceResponse, string, string, error) {
	envkeyParam, pw, envkeyHost := splitEnvkey(envkey)
	response := new(parser.EnvServiceResponse)
	err := getJson(envkeyHost, envkeyParam, options, response, fetchCache)
	return response, envkeyParam, pw, err
}

func splitEnvkey(envkey string) (string, string, string) {
	split := strings.Split(envkey, "-")
	var envkeyParam, pw, envkeyHost string
	if len(split) > 2 {
		envkeyParam, pw, envkeyHost = split[0], split[1], strings.Join(split[2:], "-")
	} else {
		envkeyParam, pw = split[0], split[1]
		envkeyHost = ""
	}

	return envkeyParam, pw, envkeyHost
}

func getBaseUrl(envkeyHost string, envkeyParam string) string {
	var host, protocol string
	if envkeyHost == "" {
		host = DefaultHost
	} else {
		host = envkeyHost
	}

	if strings.Contains(host, "localhost") {
		protocol = "http://"
	} else {
		protocol = "https://"
	}

	apiVersion := "v" + strconv.Itoa(ApiVersion)
	return strings.Join([]string{protocol + host, apiVersion, envkeyParam}, "/")
}

func getJsonUrl(envkeyHost string, envkeyParam string, options FetchOptions) string {
	baseUrl := getBaseUrl(envkeyHost, envkeyParam)

	clientName := options.ClientName
	if clientName == "" {
		clientName = "envkey-fetch"
	}

	clientVersion := options.ClientVersion
	if clientVersion == "" {
		clientVersion = version.Version
	}

	fmtStr := "%s?clientName=%s&clientVersion=%s&clientOs=%s&clientArch=%s"
	return fmt.Sprintf(
		fmtStr,
		baseUrl,
		url.QueryEscape(clientName),
		url.QueryEscape(clientVersion),
		url.QueryEscape(runtime.GOOS),
		url.QueryEscape(runtime.GOARCH),
	)
}

func getBackupUrl(envkeyParam string) string {
	host := BackupDefaultHost
	protocol := "https://"
	apiVersion := "v" + strconv.Itoa(ApiVersion)
	return strings.Join([]string{protocol + host, apiVersion, envkeyParam}, "/")
}

func getJson(envkeyHost string, envkeyParam string, options FetchOptions, response *parser.EnvServiceResponse, fetchCache *cache.Cache) error {
	var err error
	var body []byte
	var r *http.Response

	url := getJsonUrl(envkeyHost, envkeyParam, options)

	r, err = HttpGetter.Get(url)
	if r != nil {
		defer r.Body.Close()
	}

	// If http request failed and we're using the default host, now try backup host
	if err != nil || r.StatusCode >= 500 {
		if envkeyHost == "" || envkeyHost == DefaultHost {
			r, err = HttpGetter.Get(getBackupUrl(envkeyParam))
			if r != nil {
				defer r.Body.Close()
			}
		}
	}

	if err == nil && r.StatusCode == 200 {
		body, err = ioutil.ReadAll(r.Body)
		if err != nil {
			return err
		}
	} else if err != nil || r.StatusCode >= 500 {
		// try loading from cache
		if fetchCache == nil {
			if err == nil {
				return errors.New("server error.")
			} else {
				return err
			}
		} else {
			body, err = fetchCache.Read(envkeyParam)
			if err != nil {
				return errors.New("could not load from server, s3 backup, or cache.")
			}
		}

	} else if r.StatusCode == 404 {
		// Since envkey wasn't found and permission may have been removed, clear cache
		if fetchCache != nil {
			fetchCache.Delete(envkeyParam)
		}
		return errors.New("ENVKEY invalid")
	}

	err = json.Unmarshal(body, response)
	if fetchCache != nil && response.AllowCaching {
		// If caching enabled, write raw response to cache while doing decryption in parallel
		go fetchCache.Write(envkeyParam, body)
	}

	return err
}
