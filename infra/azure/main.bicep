// azd (Azure Developer CLcli) のエントリポイント。サブスクリプションスコープでリソースグループを作り、
// 各役割別モジュール (network/monitoring/security/storage/containerapps) を呼び出す (FR-OP-02, FR-OP-03)。
targetScope = 'subscription'

@minLength(1)
@maxLength(64)
@description('azd環境名。リソース命名のユニーク化に使う')
param environmentName string

@minLength(1)
@description('デプロイ先リージョン。既定は東日本 (NFR-L-01)')
@allowed(['japaneast', 'japanwest'])
param location string = 'japaneast'

@secure()
@description('ClickHouseのdefaultユーザーパスワード。初回 azd provision 時に入力を求める')
param clickhousePassword string

@secure()
@description('Query APIのAPI Key (v1.0はAPI Keyのみ, NFR-S-02)')
param queryApiKey string

@secure()
@description('MCP ServerのAPI Key')
param mcpApiKey string

// azd はコンテナをビルド・push した後、実際のイメージ参照でこれらのパラメータを上書きして再デプロイする
// (`azd deploy` / `azd up`)。初回 provision 時はプレースホルダーイメージで一旦起動させる。
param clickhouseImage string = 'mcr.microsoft.com/azuredocs/containerapps-helloworld:latest'
param ingestImage string = 'mcr.microsoft.com/azuredocs/containerapps-helloworld:latest'
param queryImage string = 'mcr.microsoft.com/azuredocs/containerapps-helloworld:latest'
param mcpImage string = 'mcr.microsoft.com/azuredocs/containerapps-helloworld:latest'

var resourceToken = toLower(uniqueString(subscription().id, environmentName, location))
var tags = {
  'azd-env-name': environmentName
  project: 'o11y-mcp'
}

resource rg 'Microsoft.Resources/resourceGroups@2025-04-01' = {
  name: 'rg-${environmentName}'
  location: location
  tags: tags
}

module monitoring 'modules/monitoring.bicep' = {
  name: 'monitoring'
  scope: rg
  params: {
    location: location
    resourceToken: resourceToken
    tags: tags
  }
}

module network 'modules/network.bicep' = {
  name: 'network'
  scope: rg
  params: {
    location: location
    resourceToken: resourceToken
    tags: tags
  }
}

module security 'modules/security.bicep' = {
  name: 'security'
  scope: rg
  params: {
    location: location
    resourceToken: resourceToken
    tags: tags
    clickhousePassword: clickhousePassword
    queryApiKey: queryApiKey
    mcpApiKey: mcpApiKey
  }
}

module storage 'modules/storage.bicep' = {
  name: 'storage'
  scope: rg
  params: {
    location: location
    resourceToken: resourceToken
    tags: tags
    managedIdentityPrincipalId: security.outputs.managedIdentityPrincipalId
  }
}

module containerApps 'modules/containerapps.bicep' = {
  name: 'containerApps'
  scope: rg
  params: {
    location: location
    resourceToken: resourceToken
    tags: tags
    subnetId: network.outputs.containerAppsSubnetId
    logAnalyticsWorkspaceId: monitoring.outputs.logAnalyticsWorkspaceId
    appInsightsConnectionString: monitoring.outputs.appInsightsConnectionString
    managedIdentityId: security.outputs.managedIdentityId
    keyVaultUri: security.outputs.keyVaultUri
    storageAccountName: storage.outputs.storageAccountName
    clickhouseFileShareName: storage.outputs.clickhouseFileShareName
    coldContainerName: storage.outputs.coldContainerName
    clickhousePassword: clickhousePassword
    clickhouseImage: clickhouseImage
    ingestImage: ingestImage
    queryImage: queryImage
    mcpImage: mcpImage
  }
}

output AZURE_RESOURCE_GROUP string = rg.name
output QUERY_API_URL string = containerApps.outputs.queryApiUrl
output MCP_SERVER_URL string = containerApps.outputs.mcpServerUrl
output INGEST_OTLP_HTTP_URL string = containerApps.outputs.ingestOtlpHttpUrl
