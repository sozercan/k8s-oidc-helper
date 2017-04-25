package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	flag "github.com/ogier/pflag"
	yaml "gopkg.in/yaml.v2"
)

const Version = "0.0.1"

var version = flag.BoolP("version", "v", false, "print version and exit")

var openBrowser = flag.BoolP("open", "o", true, "Open the oauth approval URL in the browser")

var clientIDFlag = flag.String("client-id", "", "The ClientID for the application")
var clientSecretFlag = flag.String("client-secret", "", "The ClientSecret for the application")
var appFile = flag.StringP("config", "c", "", "Path to a json file containing your application's ClientID and ClientSecret. Supercedes the --client-id and --client-secret flags.")

const oauthUrl = "https://login.microsoftonline.com/common/oauth2/authorize?client_id=%s&response_type=code&redirect_uri=https://localhost&scope=openid offline_access user.read"

type ConfigFile struct {
	Installed *MicrosoftConfig `json:"installed"`
}

type MicrosoftConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IdToken      string `json:"id_token"`
}

func readConfig(path string) (*MicrosoftConfig, error) {
	f, err := os.Open(path)
	defer f.Close()
	if err != nil {
		return nil, err
	}
	cf := &ConfigFile{}
	err = json.NewDecoder(f).Decode(cf)
	if err != nil {
		return nil, err
	}
	return cf.Installed, nil
}

// Get the id_token and refresh_token from microsoft
func getTokens(clientID, clientSecret, code string) (*TokenResponse, error) {
	val := url.Values{}
	val.Add("grant_type", "authorization_code")
	val.Add("redirect_uri", "https://localhost")
	val.Add("client_id", clientID)
	val.Add("client_secret", clientSecret)
	val.Add("code", code)
	val.Add("resource", "https://graph.microsoft.com")

	resp, err := http.PostForm("https://login.microsoftonline.com/common/oauth2/token", val)

	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}
	tr := &TokenResponse{}
	err = json.NewDecoder(resp.Body).Decode(tr)
	if err != nil {
		return nil, err
	}
	return tr, nil
}

type KubectlUser struct {
	Name         string        `yaml:"name"`
	KubeUserInfo *KubeUserInfo `yaml:"user"`
}

type KubeUserInfo struct {
	AuthProvider *AuthProvider `yaml:"auth-provider"`
}

type AuthProvider struct {
	APConfig *APConfig `yaml:"config"`
	Name     string    `yaml:"name"`
}

type APConfig struct {
	ClientID     string `yaml:"client-id"`
	ClientSecret string `yaml:"client-secret"`
	IdToken      string `yaml:"id-token"`
	IdpIssuerUrl string `yaml:"idp-issuer-url"`
	RefreshToken string `yaml:"refresh-token"`
}

type UserInfo struct {
	Mail string `json:"mail"`
}

func getUserEmail(accessToken string) (string, error) {
	client := &http.Client{}
	postData := make([]byte, 100)
	req, err := http.NewRequest("GET", "https://graph.microsoft.com/v1.0/me/", bytes.NewReader(postData))
	if err != nil {
		os.Exit(1)
	}
	req.Header.Add("Authorization", accessToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)

	defer resp.Body.Close()

	if err != nil {
		return "", err
	}
	ui := &UserInfo{}
	err = json.NewDecoder(resp.Body).Decode(ui)
	if err != nil {
		return "", err
	}
	return ui.Mail, nil
}

func generateUser(email, clientId, clientSecret, idToken, refreshToken string) *KubectlUser {
	return &KubectlUser{
		Name: email,
		KubeUserInfo: &KubeUserInfo{
			AuthProvider: &AuthProvider{
				APConfig: &APConfig{
					ClientID:     clientId,
					ClientSecret: clientSecret,
					IdToken:      idToken,
					IdpIssuerUrl: "https://login.microsoftonline.com/common/v2.0",
					RefreshToken: refreshToken,
				},
				Name: "oidc",
			},
		},
	}
}

func main() {

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *version {
		fmt.Printf("k8s-oidc-helper %s\n", Version)
		os.Exit(0)
	}

	var gcf *MicrosoftConfig
	var err error
	if len(*appFile) > 0 {
		gcf, err = readConfig(*appFile)
		if err != nil {
			fmt.Printf("Error reading config file %s: %s\n", *appFile, err)
			os.Exit(1)
		}
	}
	var clientID string
	var clientSecret string
	if gcf != nil {
		clientID = gcf.ClientID
		clientSecret = gcf.ClientSecret
	} else {
		clientID = *clientIDFlag
		clientSecret = *clientSecretFlag
	}

	if *openBrowser {
		cmd := exec.Command("open", fmt.Sprintf(oauthUrl, clientID))
		err = cmd.Start()
	}
	if !*openBrowser || err != nil {
		fmt.Printf("Open this url in your browser: %s\n", fmt.Sprintf(oauthUrl, clientID))
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter the code Microsoft gave you: ")
	code, _ := reader.ReadString('\n')
	code = strings.TrimSpace(code)

	tokResponse, err := getTokens(clientID, clientSecret, code)
	if err != nil {
		fmt.Printf("Error getting tokens: %s\n", err)
		os.Exit(1)
	}

	email, err := getUserEmail(tokResponse.AccessToken)
	if err != nil {
		fmt.Printf("Error getting user email: %s\n", err)
		os.Exit(1)
	}

	userConfig := generateUser(email, clientID, clientSecret, tokResponse.IdToken, tokResponse.RefreshToken)
	output := map[string][]*KubectlUser{}
	output["users"] = []*KubectlUser{userConfig}
	response, err := yaml.Marshal(output)
	if err != nil {
		fmt.Printf("Error marshaling yaml: %s\n", err)
		os.Exit(1)
	}
	fmt.Println("\n# Add the following to your ~/.kube/config")
	fmt.Println(string(response))
}
