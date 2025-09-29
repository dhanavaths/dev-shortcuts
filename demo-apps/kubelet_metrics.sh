TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
curl --insecure -H "Authorization: Bearer $TOKEN" https://localhost:10250/metrics
