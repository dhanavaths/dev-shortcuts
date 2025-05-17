'''
azure cli if req: curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
sudo apt-get install python3
sudo apt-get install python3-pip
pip3 install azure-identity
'''
from azure.identity import DefaultAzureCredential, ClientAssertionCredential
from azure.core.exceptions import ClientAuthenticationError
from azure.core.credentials import TokenCredential
from azure.core.credentials import AccessToken
import time

class TrustTokenService:
    def __init__(self, trust_token_service_config):
        # Set up Managed Identity credentials with specific options
        self.credential = DefaultAzureCredential(
            managed_identity_client_id=trust_token_service_config['IdentityClientID'],
            authority_host=trust_token_service_config['AuthorityHostUrl']
        )

        # Define the Token Audience (Scope) and Tenant ID
        self.token_audience = trust_token_service_config['TokenAudience']
        self.tenant_id = trust_token_service_config['TenantID']

    def get_token(self):
        try:
            # Get the token for the specified audience (scope)
            token = self.credential.get_token(self.token_audience)
            return token
        except ClientAuthenticationError as e:
            print(f"Authentication failed: {e}")
            return None

def get_service_token(federated_token):
    tenant_id=''
    client_id=''
    def cb():
        return federated_token
    cred = ClientAssertionCredential(tenant_id, client_id, cb)
    access_token = cred.get_token("https://graph.microsoft.com/.default")
    print(f'{access_token.token}')

def _main():
    # Example usage:
    trust_token_service_config = {
        'IdentityClientID': '',
        'AuthorityHostUrl': 'https://management.azure.com//.default',
        'TokenAudience': 'api://AzureADTokenExchange',
        'TenantID': ''
    }

    service = TrustTokenService(trust_token_service_config)
    token = service.get_token()

    if token:
        print(f"{token.token}")
    else:
        print("Failed to retrieve the token.")
        return
    get_service_token(token.token)

if __name__ == '__main__':
    _main()

