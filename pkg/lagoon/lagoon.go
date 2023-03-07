package lagoon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/interceptor"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	"github.com/salsadigitalauorg/rockpool/pkg/ssh"

	"github.com/shurcooL/graphql"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

var DefaultVersion = "v2.12.0"

// Version is the version of Lagoon to be installed.
var Version string

var GqlClient *graphql.Client

var Remotes []Remote

var lagoonUserinfo struct {
	Me struct {
		Id      graphql.String
		Email   graphql.String
		SshKeys []struct {
			Name           string
			KeyFingerprint string
		}
	}
}

// FetchApiAdminToken creates an admin token with superpowers.
// See https://docs.lagoon.sh/administering-lagoon/graphql-queries/#running-graphql-queries
func FetchApiAdminToken() string {
	log.Info("fetching lagoon api admin token")
	out, err := kube.Exec(
		platform.ControllerClusterName(), "lagoon-core",
		"lagoon-core-storage-calculator", "/create_jwt.py").Output()
	if err != nil {
		log.WithError(command.GetMsgFromCommandError(err)).Panic()
	}
	return string(out)
}

func FetchApiToken() string {
	log.Info("fetching lagoon api token")
	_, password := kube.GetSecret(platform.ControllerClusterName(),
		"lagoon-core",
		"lagoon-core-keycloak",
		"KEYCLOAK_LAGOON_ADMIN_PASSWORD",
	)

	data := url.Values{
		"client_id":  {"lagoon-ui"},
		"grant_type": {"password"},
		"username":   {"lagoonadmin"},
		"password":   {password},
	}
	url := fmt.Sprintf("http://keycloak.lagoon.%s/auth/realms/lagoon/protocol/openid-connect/token", platform.Hostname())
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(data.Encode()))
	if err != nil {
		log.WithFields(log.Fields{
			"url":  url,
			"data": data,
		}).WithError(err).Panic("error preparing request to token endpoint")
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	dump, _ := httputil.DumpRequest(req, true)

	client := &http.Client{
		Transport: interceptor.New(),
	}
	resp, err := client.Do(req)
	if err != nil {
		log.WithField("request", string(dump)).WithError(err).
			Panic("error executing request to token endpoint")
	}
	dump, _ = httputil.DumpResponse(resp, true)

	var res struct {
		Token            string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		log.WithField("response", string(dump)).WithError(err).
			Fatal("error parsing Lagoon API token")
	}
	if res.Error != "" {
		log.WithFields(log.Fields{
			"res.Error":            res.Error,
			"res.ErrorDescription": res.ErrorDescription,
		}).Fatal("error fetching Lagoon API token")
	}
	return res.Token
}

func InitApiClient() {
	if GqlClient != nil {
		return
	}
	src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: FetchApiToken()})
	httpClient := &http.Client{
		Transport: &oauth2.Transport{
			Base:   interceptor.New(),
			Source: oauth2.ReuseTokenSource(nil, src),
		},
	}
	GqlClient = graphql.NewClient(fmt.Sprintf("http://api.lagoon.%s/graphql", platform.Hostname()), httpClient)
}

func GetRemotes() {
	log.Info("fetching lagoon api remotes")
	var query struct {
		AllKubernetes []Remote
	}
	err := GqlClient.Query(context.Background(), &query, nil)
	if err != nil {
		log.WithError(err).Fatal("error fetching Lagoon remotes")
	}
	Remotes = query.AllKubernetes
}

func FetchUserInfo() {
	log.Info("fetching lagoon user info")
	err := GqlClient.Query(context.Background(), &lagoonUserinfo, nil)
	if err != nil {
		log.WithError(err).Fatal("error fetching Lagoon user info")
	}
}

func AddSshKey() {
	log.Info("adding ssh key for lagoon user")

	keyValue, keyType, keyFingerpint, cmt := ssh.GetPublicKeyFingerprint()
	FetchUserInfo()
	for _, k := range lagoonUserinfo.Me.SshKeys {
		if k.KeyFingerprint == keyFingerpint {
			log.Debug("lagoon ssh key had already been added")
			return
		}
	}

	var m struct {
		AddSshKey struct {
			Name           string
			KeyFingerprint string
		} `graphql:"addSshKey(input: {keyType: $keyType, keyValue: $keyValue, name: $name, user: {email: $userEmail, id: $userId}})"`
	}

	type SshKeyType string
	vars := map[string]interface{}{
		"keyType":   SshKeyType(strings.ReplaceAll(strings.ToUpper(keyType), "-", "_")),
		"keyValue":  graphql.String(keyValue),
		"name":      graphql.String(cmt),
		"userEmail": graphql.String(lagoonUserinfo.Me.Email),
		"userId":    graphql.String(lagoonUserinfo.Me.Id),
	}
	err := GqlClient.Mutate(context.Background(), &m, vars)
	if err != nil {
		log.WithField("vars", vars).WithError(err).
			Fatal("error adding Lagoon ssh key")
	}
}

func AddRemote(re Remote, token string) {
	log.Info("adding lagoon remote to GraphQL API")
	var m struct {
		AddKubernetes struct {
			Remote
		} `graphql:"addKubernetes(input: {id: $id, name: $name, consoleUrl: $console, token: $token, routerPattern: $routePattern})"`
	}
	vars := map[string]interface{}{
		"id":           graphql.Int(re.Id),
		"name":         graphql.String(re.Name),
		"console":      graphql.String(re.ConsoleUrl),
		"token":        graphql.String(token),
		"routePattern": graphql.String(re.RouterPattern),
	}
	err := GqlClient.Mutate(context.Background(), &m, vars)
	if err != nil {
		log.WithField("vars", vars).WithError(err).
			Fatal("error adding Lagoon remote", re.Name, err)
	}
	Remotes = append(Remotes, m.AddKubernetes.Remote)
}
