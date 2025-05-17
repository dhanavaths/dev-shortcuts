# curl -H "Metadata: true" "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/"
export AZURE_SUBSCRIPTION_ID="aa4b32f6-e417-4ee2-91d6-9705db5ebfcd"
export AZURE_TENANT_ID="3fa4e7ca-cbda-4e5b-b56c-bb9aa25cc2e4"
export AZURE_CLIENT_ID="b1afebc2-2390-470a-8a14-0fc621ecd35f"
export LOCAL_TESTING_MI_FIC_TOKEN=""
export AZURE_CLIENT_SECRET=""
export AZURE_ENVIRONMENT_NAME="AzurePublicCloud"
export TEST_RUN_LOCATION="centralus"
go test /root/workspace/clusterfleet/internal/provisioners/aksingress/... -count=1 -timeout 75m
