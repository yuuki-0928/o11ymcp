// コンテナレジストリモジュール (FR-OP-03)。
// azd がビルドした各サービスのイメージをpushし、Container AppsがManaged Identity経由でpullする。
// 個人開発/PoC規模のためBasic SKU (NFR-C-01)。Adminユーザーは無効にし、
// pull/pushともにAzure AD (Managed Identity) 経由のみに限定する (NFR-S-03)。
param location string
param resourceToken string
param tags object
param managedIdentityPrincipalId string

// AcrPull (組み込みロール)。Container AppsのManaged Identityにこれだけを付与し、
// レジストリへのpush権限は渡さない (azd自体はユーザーのAzure ADクレデンシャルでpushする)。
var acrPullRoleId = '7f951dda-4ed3-4680-a7ca-43fe172d538d'

resource registry 'Microsoft.ContainerRegistry/registries@2023-11-01-preview' = {
  name: 'acr${resourceToken}'
  location: location
  tags: tags
  sku: {
    name: 'Basic'
  }
  properties: {
    adminUserEnabled: false
  }
}

resource acrPullRoleAssignment 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(registry.id, managedIdentityPrincipalId, acrPullRoleId)
  scope: registry
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', acrPullRoleId)
    principalId: managedIdentityPrincipalId
    principalType: 'ServicePrincipal'
  }
}

output loginServer string = registry.properties.loginServer
output registryId string = registry.id
output registryName string = registry.name
