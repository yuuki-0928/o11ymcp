// ストレージモジュール (FR-OP-03)。
// - Blob Container: ClickHouseのコールドストレージ (FR-ST-03/05)
// - File Share: ClickHouse (Container Apps) の永続ボリューム (ホットデータ, FR-ST-05)
param location string
param resourceToken string
param tags object
param managedIdentityPrincipalId string

// ストレージアカウント名は小文字英数字のみ・24文字以内という制約があるため、
// resourceToken (uniqueStringの13文字程度) をそのまま使う。
var storageAccountName = 'st${resourceToken}'

resource storageAccount 'Microsoft.Storage/storageAccounts@2023-05-01' = {
  name: storageAccountName
  location: location
  tags: tags
  sku: {
    // NFR-L-02: Cross-Region転送(Geo-Replication)は既定OFFのため LRS。
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
  properties: {
    minimumTlsVersion: 'TLS1_2'
    allowBlobPublicAccess: false
    accessTier: 'Cool'
  }
}

resource blobService 'Microsoft.Storage/storageAccounts/blobServices@2023-05-01' = {
  parent: storageAccount
  name: 'default'
}

resource coldContainer 'Microsoft.Storage/storageAccounts/blobServices/containers@2023-05-01' = {
  parent: blobService
  name: 'o11y-mcp-cold'
  properties: {
    publicAccess: 'None'
  }
}

resource fileService 'Microsoft.Storage/storageAccounts/fileServices@2023-05-01' = {
  parent: storageAccount
  name: 'default'
}

resource clickhouseFileShare 'Microsoft.Storage/storageAccounts/fileServices/shares@2023-05-01' = {
  parent: fileService
  name: 'clickhouse-data'
  properties: {
    // FR-ST-05: 後から容量拡張可能。Bicep再適用でこの値を上げるだけで拡張できる。
    shareQuota: 100
  }
}

// 現状 ClickHouse の azure_blob_storage ディスクはアカウントキー認証を使うため必須ではないが、
// 将来 Managed Identity 経由のアクセスに切り替える選択肢を残すため付与しておく。
var storageBlobDataContributorRoleId = 'ba92f5b4-2d11-453d-a403-e96b0029c9fe'
resource blobRoleAssignment 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(storageAccount.id, managedIdentityPrincipalId, storageBlobDataContributorRoleId)
  scope: storageAccount
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', storageBlobDataContributorRoleId)
    principalId: managedIdentityPrincipalId
    principalType: 'ServicePrincipal'
  }
}

output storageAccountName string = storageAccount.name
output coldContainerName string = coldContainer.name
output clickhouseFileShareName string = clickhouseFileShare.name
