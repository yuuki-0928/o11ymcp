// セキュリティモジュール (FR-OP-03)。
// シークレットはコードに埋め込まず Key Vault で管理し、Container Apps は
// User Assigned Managed Identity 経由で読み出す (NFR-S-03)。
param location string
param resourceToken string
param tags object

@secure()
param clickhousePassword string
@secure()
param queryApiKey string
@secure()
param mcpApiKey string

// Key Vault Secrets User (組み込みロール)。Container Appsの管理IDにこれだけを付与し、
// シークレットの読み取り以外の権限は渡さない (最小権限の原則)。
var keyVaultSecretsUserRoleId = '4633458b-17de-408a-b874-0445c86b69e6'

resource managedIdentity 'Microsoft.ManagedIdentity/userAssignedIdentities@2024-11-30' = {
  name: 'id-${resourceToken}'
  location: location
  tags: tags
}

resource keyVault 'Microsoft.KeyVault/vaults@2024-11-01' = {
  name: 'kv-${resourceToken}'
  location: location
  tags: tags
  properties: {
    sku: {
      family: 'A'
      name: 'standard'
    }
    tenantId: subscription().tenantId
    enableRbacAuthorization: true
    // 個人開発/PoC規模のため論理削除の保持期間は既定最小、パージ保護はオフ (azd down で確実に消せるように)。
    enableSoftDelete: true
    softDeleteRetentionInDays: 7
    // false にすると、azd down で Key Vault が削除される際に、論理削除状態のまま残ってしまう。
    // その場合、azd up で同じ名前の Key Vault を作ろうとすると、論理削除状態の Key Vault が残っているために作成に失敗する。
    // そのため、パージ保護は有効にしておくこととする。
    enablePurgeProtection: true
  }
}

resource kvRoleAssignment 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(keyVault.id, managedIdentity.id, keyVaultSecretsUserRoleId)
  scope: keyVault
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', keyVaultSecretsUserRoleId)
    principalId: managedIdentity.properties.principalId
    principalType: 'ServicePrincipal'
  }
}

resource secretClickhousePassword 'Microsoft.KeyVault/vaults/secrets@2024-11-01' = {
  parent: keyVault
  name: 'clickhouse-password'
  properties: {
    value: clickhousePassword
  }
}

resource secretQueryApiKey 'Microsoft.KeyVault/vaults/secrets@2024-11-01' = {
  parent: keyVault
  name: 'query-api-key'
  properties: {
    value: queryApiKey
  }
}

resource secretMcpApiKey 'Microsoft.KeyVault/vaults/secrets@2024-11-01' = {
  parent: keyVault
  name: 'mcp-api-key'
  properties: {
    value: mcpApiKey
  }
}

output managedIdentityId string = managedIdentity.id
output managedIdentityPrincipalId string = managedIdentity.properties.principalId
output managedIdentityClientId string = managedIdentity.properties.clientId
output keyVaultUri string = keyVault.properties.vaultUri
output keyVaultName string = keyVault.name
