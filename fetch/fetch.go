package fetch

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
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
	multierror "github.com/hashicorp/go-multierror"
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
var BackupHost = "s3-eu-west-1.amazonaws.com/envkey-backup/envs"
var BackupHostRestricted = "me66hg5t17.execute-api.eu-west-1.amazonaws.com/default/envBackup"
var ApiVersion = 1

var Client *http.Client

type httpChannelResponse struct {
	response *http.Response
	url      string
}

type httpChannelErr struct {
	err error
	url string
}

func Fetch(envkey string, options FetchOptions) (string, error) {
	if len(strings.Split(envkey, "-")) < 2 {
		return "", errors.New("ENVKEY invalid")
	}

	// may be initalized already when mocking for tests
	if Client == nil {
		InitHttpClient(options.TimeoutSeconds)
	}

	var fetchCache *cache.Cache
	var cacheErr error

	if options.ShouldCache {
		if options.VerboseOutput {
			fmt.Fprintf(os.Stderr, "Initializing cache at %s", options.CacheDir)
		}

		// If initializing cache fails for some reason, ignore and let it be nil
		fetchCache, cacheErr = cache.NewCache(options.CacheDir)

		if options.VerboseOutput {
			fmt.Fprintf(os.Stderr, "Error initializing cache: %s", cacheErr.Error())
		}
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

func UrlWithLoggingParams(baseUrl string, options FetchOptions) string {
	clientName := options.ClientName
	if clientName == "" {
		clientName = "envkey-fetch"
	}

	clientVersion := options.ClientVersion
	if clientVersion == "" {
		clientVersion = version.Version
	}

	var querySep string
	if strings.Contains(baseUrl, "?") {
		querySep = "&"
	} else {
		querySep = "?"
	}

	fmtStr := "%s%sclientName=%s&clientVersion=%s&clientOs=%s&clientArch=%s"
	return fmt.Sprintf(
		fmtStr,
		baseUrl,
		querySep,
		url.QueryEscape(clientName),
		url.QueryEscape(clientVersion),
		url.QueryEscape(runtime.GOOS),
		url.QueryEscape(runtime.GOARCH),
	)
}

func InitHttpClient(timeoutSeconds float64) {
	// http.Client.Get reuses the transport. this should be created once.
	tp := http.Transport{}
	to := time.Second * time.Duration(timeoutSeconds)

	tp.DialContext = (&net.Dialer{
		Timeout: to,
	}).DialContext

	tp.TLSHandshakeTimeout = to
	tp.ResponseHeaderTimeout = to
	tp.ExpectContinueTimeout = to

	Client = &http.Client{
		Transport: &tp,
	}
}

func httpExecRequest(
	req *http.Request,
	respChan chan httpChannelResponse,
	errChan chan httpChannelErr,
) {
	resp, err := Client.Do(req)
	if err == nil {
		respChan <- httpChannelResponse{resp, req.URL.String()}
	} else {
		// if error caused by missing root certificates, pull in gocertifi certs (which come from Mozilla) and try again with those
		if strings.Contains(err.Error(), "x509: failed to load system roots") {
			certPool, certPoolErr := gocertifi.CACerts()
			if certPoolErr != nil {
				errChan <- httpChannelErr{multierror.Append(err, certPoolErr), req.URL.String()}
				return
			}
			Client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{RootCAs: certPool}
			httpExecRequest(req, respChan, errChan)
		} else {
			errChan <- httpChannelErr{err, req.URL.String()}
		}
	}
}

func httpGetAsync(
	url string,
	ctx context.Context,
	respChan chan httpChannelResponse,
	errChan chan httpChannelErr,
) {
	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		errChan <- httpChannelErr{err, url}
		return
	}

	req = req.WithContext(ctx)

	go httpExecRequest(req, respChan, errChan)
}

func httpGet(url string) (*http.Response, error) {
	respChan, errChan := make(chan httpChannelResponse), make(chan httpChannelErr)

	httpGetAsync(url, context.Background(), respChan, errChan)

	for {
		select {
		case channelResp := <-respChan:
			return channelResp.response, nil
		case channelErr := <-errChan:
			return nil, channelErr.err
		}
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

	hostSplit := strings.Split(host, ":")

	if len(hostSplit) > 0 && hostSplit[0] == "localhost" {
		protocol = "http://"
	} else {
		protocol = "https://"
	}

	apiVersion := "v" + strconv.Itoa(ApiVersion)
	return strings.Join([]string{protocol + host, apiVersion, envkeyParam}, "/")
}

func getJsonUrl(envkeyHost string, envkeyParam string, options FetchOptions) string {
	baseUrl := getBaseUrl(envkeyHost, envkeyParam)
	return UrlWithLoggingParams(baseUrl, options)
}

func getBackupUrls(envkeyParam string) []string {
	protocol := "https://"
	apiVersion := strconv.Itoa(ApiVersion)
	return []string{
		strings.Join([]string{protocol + BackupHost, "v" + apiVersion, envkeyParam}, "/"),
		fmt.Sprintf("%s?v=%s&id=%s", protocol+BackupHostRestricted, apiVersion, envkeyParam),
	}
}

func fetchBackup(envkeyParam string, options FetchOptions) (*http.Response, error) {
	backupUrls := getBackupUrls(envkeyParam)

	if options.VerboseOutput {
		fmt.Fprintf(os.Stderr, "Attempting to load encrypted config from backup urls: %s\n", backupUrls)
	}

	respChan, errChan := make(chan httpChannelResponse), make(chan httpChannelErr)

	cancelFnByUrl := map[string]context.CancelFunc{}

	for _, backupUrl := range backupUrls {
		ctx, cancel := context.WithCancel(context.Background())
		urlWithParams := UrlWithLoggingParams(backupUrl, options)
		cancelFnByUrl[urlWithParams] = cancel
		httpGetAsync(urlWithParams, ctx, respChan, errChan)
	}

	var err error
	numErrs := 0
	for {
		select {
		case channelResp := <-respChan:
			logRequestIfVerbose(channelResp.url, options, nil, channelResp.response)

			// cancel other requests
			for backupUrl, cancel := range cancelFnByUrl {
				if backupUrl != channelResp.url {
					cancel()
				}
			}

			return channelResp.response, nil
		case channelErr := <-errChan:
			err = multierror.Append(err, channelErr.err)
			numErrs++
			if numErrs == len(backupUrls) {
				logRequestIfVerbose(channelErr.url, options, channelErr.err, nil)
				return nil, err
			}
		}
	}
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

	// If http request failed and we're using the default host, now try backup hosts
	if fetchErr != nil || r.StatusCode >= 500 {
		logRequestIfVerbose(url, options, fetchErr, r)

		if envkeyHost == "" || envkeyHost == DefaultHost {
			r, backupFetchErr = fetchBackup(envkeyParam, options)

			if r != nil {
				defer r.Body.Close()
			}
		}
	}

	if backupFetchErr == nil && (r != nil && r.StatusCode == 200) {
		body, err = ioutil.ReadAll(r.Body)

		if err != nil {
			if options.VerboseOutput {
				fmt.Fprintln(os.Stderr, "Error reading response body:")
				fmt.Fprintln(os.Stderr, err)
			}
			return err
		}
	} else if backupFetchErr != nil || (r != nil && r.StatusCode >= 500) {
		// try loading from cache
		if fetchCache == nil {
			if backupFetchErr == nil {
				return errors.New("could not load from server or s3 backup.")
			} else {
				return errors.New("could not load from server or s3 backup.\nfetch error: " + fetchErr.Error() + "\nbackup fetch error: " + backupFetchErr.Error())
			}
		} else {
			body, err = fetchCache.Read(envkeyParam)
			if err != nil {
				if options.VerboseOutput {
					fmt.Fprintln(os.Stderr, "Cache read error:")
					fmt.Fprintln(os.Stderr, err)
				}
				return errors.New("could not load from server, s3 backup, or cache.\nfetch error: " + fetchErr.Error() + "\nbackup fetch error: " + backupFetchErr.Error() + "\ncache read error: " + err.Error())
			}
		}

	} else if r != nil && r.StatusCode == 404 {
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
