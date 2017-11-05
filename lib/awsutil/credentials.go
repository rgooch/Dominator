package awsutil

import (
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
)

type sessionResult struct {
	accountName string
	accountId   string
	awsSession  *session.Session
	regions     []string
	err         error
}

func tryLoadCredentials() (*CredentialsStore, map[string]error, error) {
	accountNames, err := listAccountNames()
	if err != nil {
		return nil, nil, err
	}
	cs, unloadableAccounts := createCredentials(accountNames)
	return cs, unloadableAccounts, nil
}

func loadCredentials() (*CredentialsStore, error) {
	accountNames, err := listAccountNames()
	if err != nil {
		return nil, err
	}
	cs, unloadableAccounts := createCredentials(accountNames)
	for _, err := range unloadableAccounts {
		return nil, err
	}
	return cs, nil
}

func createCredentials(accountNames []string) (
	*CredentialsStore, map[string]error) {
	sort.Strings(accountNames)
	cs := &CredentialsStore{
		accountNames:    accountNames,
		sessionMap:      make(map[string]*session.Session),
		accountIdToName: make(map[string]string),
		accountNameToId: make(map[string]string),
		accountRegions:  make(map[string][]string),
	}
	resultsChannel := make(chan sessionResult, len(accountNames))
	for _, accountName := range accountNames {
		go func(accountName string) {
			resultsChannel <- createSession(accountName)
		}(accountName)
	}
	unloadableAccounts := make(map[string]error)
	for range accountNames {
		result := <-resultsChannel
		if result.err != nil {
			unloadableAccounts[result.accountName] = result.err
		} else {
			cs.sessionMap[result.accountName] = result.awsSession
			cs.accountIdToName[result.accountId] = result.accountName
			cs.accountNameToId[result.accountName] = result.accountId
			cs.accountRegions[result.accountName] = result.regions
		}
	}
	close(resultsChannel)
	return cs, unloadableAccounts
}

func createSession(accountName string) sessionResult {
	awsSession, err := CreateSession(accountName)
	if err != nil {
		return sessionResult{err: err, accountName: accountName}
	}
	stsService := sts.New(awsSession)
	inp := &sts.GetCallerIdentityInput{}
	var accountId string
	if out, err := stsService.GetCallerIdentity(inp); err != nil {
		return sessionResult{err: err, accountName: accountName}
	} else {
		if arnV, err := arn.Parse(aws.StringValue(out.Arn)); err != nil {
			return sessionResult{err: err, accountName: accountName}
		} else {
			accountId = arnV.AccountID
		}
	}
	regions, err := listRegions(CreateService(awsSession, "us-east-1"))
	if err != nil {
		return sessionResult{err: err, accountName: accountName}
	}
	return sessionResult{
		accountName: accountName,
		accountId:   accountId,
		awsSession:  awsSession,
		regions:     regions,
	}
}