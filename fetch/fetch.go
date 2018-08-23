package fetch

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/certifi/gocertifi"
	"github.com/envkey/envkey-fetch/cache"
	"github.com/envkey/envkey-fetch/parser"
	"github.com/envkey/envkey-fetch/version"
	"github.com/envkey/myhttp"
)

type FetchOptions struct {
	ShouldCache    bool
	CacheDir       string
	ClientName     string
	ClientVersion  string
	VerboseOutput  bool
	TimeoutSeconds float64
}

var DefaultHost = "env.envkey.com"
var BackupDefaultHost = "s3-eu-west-1.amazonaws.com/envkey-backup/envs"
var ApiVersion = 1
var HttpGetter = myhttp.New(time.Second * 2)

func Fetch(envkey string, options FetchOptions) (string, error) {
	if len(strings.Split(envkey, "-")) < 2 {
		return "", errors.New("ENVKEY invalid")
	}

	if options.TimeoutSeconds != 2.0 {
		HttpGetter = myhttp.New(time.Second * time.Duration(options.TimeoutSeconds))
	}

	var fetchCache *cache.Cache

	if options.ShouldCache {
		// If initializing cache fails for some reason, ignore and let it be nil
		fetchCache, _ = cache.NewCache(options.CacheDir)
	}

	response, envkeyParam, pw, err := fetchEnv(envkey, options, fetchCache)
	if err != nil {
		return "", err
	}

	if options.VerboseOutput {
		fmt.Fprintln(os.Stderr, "Parsing and decrypting response...")
	}
	res, err := response.Parse(pw)
	if err != nil {
		if options.VerboseOutput {
			fmt.Fprintln(os.Stderr, "Error parsing and decrypting:")
			fmt.Fprintln(os.Stderr, err)
		}

		if fetchCache != nil {
			fetchCache.Delete(envkeyParam)
		}
		return "", errors.New("ENVKEY invalid")
	}

	// Ensure cache bizness finished (don't worry about error)
	if fetchCache != nil {
		select {
		case <-fetchCache.Done:
		default:
		}
	}

	return res, nil
}

func httpGet(url string) (*http.Response, error) {
	r, err := HttpGetter.Get(url)

	// if error caused by missing root certificates, pull in gocertifi certs (which come from Mozilla) and try again with those
	if err != nil && strings.Contains(err.Error(), "x509: failed to load system roots") {
		certPool, err := gocertifi.CACerts()
		if err != nil {
			return nil, err
		}
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: certPool},
		}
		HttpGetter.Client.Transport = transport
		return HttpGetter.Get(url)
	} else {
		return r, err
	}
}

func logRequestIfVerbose(url string, options FetchOptions, err error, r *http.Response) {
	if options.VerboseOutput {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Loading from %s failed.\n", url)
			fmt.Fprintln(os.Stderr, "Error:")
			fmt.Fprintln(os.Stderr, err)
		} else if r.StatusCode >= 500 {
			fmt.Fprintf(os.Stderr, "Loading from %s failed.\n", url)
			fmt.Fprintln(os.Stderr, "Response status:")
			fmt.Fprintln(os.Stderr, string(r.StatusCode))
		} else {
			fmt.Fprintf(os.Stderr, "Loaded from %s successfully.\n", url)
		}
	}
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
	var err, fetchErr, backupFetchErr error
	var body []byte
	var r *http.Response

	url := getJsonUrl(envkeyHost, envkeyParam, options)

	r, fetchErr = httpGet(url)
	if r != nil {
		defer r.Body.Close()
	}

	if options.VerboseOutput {
		fmt.Fprintf(os.Stderr, "Attempting to load encrypted config from default url: %s\n", url)
	}

	// If http request failed and we're using the default host, now try backup host
	if fetchErr != nil || r.StatusCode >= 500 {
		logRequestIfVerbose(url, options, fetchErr, r)

		if envkeyHost == "" || envkeyHost == DefaultHost {
			backupUrl := getBackupUrl(envkeyParam)

			if options.VerboseOutput {
				fmt.Fprintf(os.Stderr, "Attempting to load encrypted config from backup url: %s\n", backupUrl)
			}

			r, backupFetchErr = httpGet(backupUrl)
			if r != nil {
				defer r.Body.Close()
			}

			logRequestIfVerbose(backupUrl, options, backupFetchErr, r)
		}
	}

	if backupFetchErr == nil && r.StatusCode == 200 {
		body, err = ioutil.ReadAll(r.Body)
		if err != nil {
			if options.VerboseOutput {
				fmt.Fprintln(os.Stderr, "Error reading response body:")
				fmt.Fprintln(os.Stderr, err)
			}
			return err
		}
	} else if backupFetchErr != nil || r.StatusCode >= 500 {
		// try loading from cache
		if fetchCache == nil {
			if backupFetchErr == nil {
				return errors.New("could not load from server or s3 backup.")
			} else {
				return errors.New("could not load from server or s3 backup.\n\nfetch error: " + fetchErr.Error() + "\n\nbackup fetch error: " + backupFetchErr.Error())
			}
		} else {
			body, err = fetchCache.Read(envkeyParam)
			if err != nil {
				if options.VerboseOutput {
					fmt.Fprintln(os.Stderr, "Cache read error:")
					fmt.Fprintln(os.Stderr, err)
				}
				return errors.New("could not load from server, s3 backup, or cache.\n\nfetch error: " + fetchErr.Error() + "\n\nbackup fetch error: " + backupFetchErr.Error() + "\n\ncache read error:" + err.Error())
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
