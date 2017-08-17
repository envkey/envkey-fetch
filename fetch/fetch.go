package fetch

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/envkey/envkey-fetch/cache"
	"github.com/envkey/envkey-fetch/parser"
)

type FetchOptions struct {
	ShouldCache bool
	CacheDir    string
}

const DefaultHost = "env-service.herokuapp.com"
const ApiVersion = 1

func Fetch(envkey string, options FetchOptions) string {
	var fetchCache *cache.Cache

	if options.ShouldCache {
		// If initializing cache fails for some reason, ignore and let it be nil
		fetchCache, _ = cache.NewCache(options.CacheDir)
	}

	response, envkeyParam, pw, err := fetchEnv(envkey, fetchCache)
	if err != nil {
		return ""
	}
	res, err := response.Parse(pw)
	if err != nil {
		if fetchCache != nil {
			fetchCache.Delete(envkeyParam)
		}
		return ""
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

func fetchEnv(envkey string, fetchCache *cache.Cache) (*parser.EnvServiceResponse, string, string, error) {
	envkeyParam, pw, envkeyHost := splitEnvkey(envkey)
	response := new(parser.EnvServiceResponse)
	err := getJson(envkeyHost, envkeyParam, response, fetchCache)
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

func getJsonUrl(envkeyHost string, envkeyParam string) string {
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

	version := "v" + strconv.Itoa(ApiVersion)

	return strings.Join([]string{protocol + host, version, envkeyParam}, "/")
}

func getJson(envkeyHost string, envkeyParam string, response *parser.EnvServiceResponse, fetchCache *cache.Cache) error {
	var err error
	var body []byte

	url := getJsonUrl(envkeyHost, envkeyParam)
	r, err := http.Get(url)
	if r != nil {
		defer r.Body.Close()
	}

	if err == nil && r.StatusCode == 200 {
		body, err = ioutil.ReadAll(r.Body)
		if err != nil {
			return err
		}
	} else if err != nil || r.StatusCode == 500 {
		// Since http request failed, try loading from cache
		if fetchCache == nil {
			if err == nil {
				return errors.New("Server error.")
			} else {
				return err
			}
		} else {
			body, err = fetchCache.Read(envkeyParam)
			if err != nil {
				return err
			}
		}

	} else if r.StatusCode == 404 {
		// Since envkey wasn't find and permission may have been removed, clear cache
		if fetchCache != nil {
			fetchCache.Delete(envkeyParam)
		}
		return errors.New("Envkey not found")
	}

	err = json.Unmarshal(body, response)
	if fetchCache != nil && response.AllowCaching {
		// If caching enabled, write raw response to cache while doing decryption in parallel
		go fetchCache.Write(envkeyParam, body)
	}

	return err
}
