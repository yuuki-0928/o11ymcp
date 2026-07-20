// Container Apps モジュール (FR-OP-03)。環境本体と4つのアプリ (clickhouse/ingest/query/mcp) を定義する。
param location string
param resourceToken string
param tags object
param subnetId string
param logAnalyticsWorkspaceId string
param appInsightsConnectionString string
param managedIdentityId string
param keyVaultUri string
param storageAccountName string
param clickhouseFileShareName string
param coldContainerName string

param clickhouseImage string
param ingestImage string
param queryImage string
param mcpImage string

@secure()
param clickhousePassword string

// Key Vaultのシークレット参照URL (Container Appsの secretRef 経由で読む)。
// FR-OP-05: Azure/Compose両方で同一OCIイメージを使う方針のため、
// ここで組み立てる各種URLはAzure固有の配線であり、アプリ本体のコードには一切依存しない。
var clickhousePasswordSecretUrl = '${keyVaultUri}secrets/clickhouse-password'
var queryApiKeySecretUrl = '${keyVaultUri}secrets/query-api-key'
var mcpApiKeySecretUrl = '${keyVaultUri}secrets/mcp-api-key'

resource logAnalytics 'Microsoft.OperationalInsights/workspaces@2023-09-01' existing = {
  name: last(split(logAnalyticsWorkspaceId, '/'))
}

resource storageAccount 'Microsoft.Storage/storageAccounts@2023-05-01' existing = {
  name: storageAccountName
}

resource containerAppEnv 'Microsoft.App/managedEnvironments@2024-03-01' = {
  name: 'cae-${resourceToken}'
  location: location
  tags: tags
  properties: {
    appLogsConfiguration: {
      destination: 'log-analytics'
      logAnalyticsConfiguration: {
        customerId: logAnalytics.properties.customerId
        sharedKey: logAnalytics.listKeys().primarySharedKey
      }
    }
    vnetConfiguration: {
      infrastructureSubnetId: subnetId
    }
  }
}

resource clickhouseStorage 'Microsoft.App/managedEnvironments/storages@2024-03-01' = {
  parent: containerAppEnv
  name: 'clickhouse-data'
  properties: {
    azureFile: {
      accountName: storageAccountName
      accountKey: storageAccount.listKeys().keys[0].value
      shareName: clickhouseFileShareName
      accessMode: 'ReadWrite'
    }
  }
}

// --- ClickHouse ---
// 単一レプリカ固定 (minReplicas=maxReplicas=1)。MergeTreeは単純な水平スケールができず、
// Azure Files 1本を複数レプリカで同時書き込みすると壊れるため。
// スタック中で唯一「常時起動」するコンポーネント (コスト最小化 NFR-C-01 上の主なトレードオフ)。
resource clickhouseApp 'Microsoft.App/containerApps@2024-03-01' = {
  name: 'ca-clickhouse-${resourceToken}'
  location: location
  tags: union(tags, { 'azd-service-name': 'clickhouse' })
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${managedIdentityId}': {}
    }
  }
  properties: {
    managedEnvironmentId: containerAppEnv.id
    configuration: {
      // ClickHouseはIngest/Query APIからのみ参照される内部コンポーネントのため外部公開しない。
      ingress: {
        external: false
        transport: 'tcp'
        targetPort: 9000
      }
      // 非HTTPのスケールルール (tcp) を使う場合、Microsoft Learn (scale-app) の記載により
      // activeRevisionsMode を single にする必要がある。
      activeRevisionsMode: 'single'
      secrets: [
        { name: 'clickhouse-password', keyVaultUrl: clickhousePasswordSecretUrl, identity: managedIdentityId }
        { name: 'storage-account-key', value: storageAccount.listKeys().keys[0].value }
      ]
    }
    template: {
      // NFR-C-01 (常時起動リソースの最小化)。当初 minReplicas=1 (常時起動) にしていたが、
      // Azure Retail Prices API で試算した結果 1vCPU/2GiB 24時間稼働だけで月$70超と
      // コスト目標(月5,000円)を大幅に超過することが判明したため scale-to-zero に変更した。
      // トレードオフ: 0台からの起動時にコールドスタート(数十秒程度)が発生し、その間は
      // Ingest/Query からの接続が失敗し得る (要件定義書 R-03 相当のリスクとして許容する)。
      // データはコンテナではなく Azure Files (clickhouseStorage) 側にあるため、
      // 0台に縮退してもデータ自体は失われない。
      scale: {
        minReplicas: 0
        maxReplicas: 1
        rules: [
          {
            name: 'tcp-scaling'
            tcp: {
              metadata: {
                concurrentConnections: '10'
              }
            }
          }
        ]
      }
      volumes: [
        {
          name: 'clickhouse-data'
          storageType: 'AzureFile'
          storageName: clickhouseStorage.name
        }
      ]
      containers: [
        {
          name: 'clickhouse'
          image: clickhouseImage
          resources: {
            cpu: json('1.0')
            memory: '2Gi'
          }
          volumeMounts: [
            { volumeName: 'clickhouse-data', mountPath: '/var/lib/clickhouse' }
          ]
          env: [
            { name: 'CLICKHOUSE_USER', value: 'default' }
            { name: 'CLICKHOUSE_PASSWORD', secretRef: 'clickhouse-password' }
            { name: 'CLICKHOUSE_DB', value: 'default' }
            { name: 'AZURE_STORAGE_ACCOUNT_URL', value: 'https://${storageAccountName}.blob.core.windows.net' }
            { name: 'AZURE_STORAGE_CONTAINER_NAME', value: coldContainerName }
            { name: 'AZURE_STORAGE_ACCOUNT_NAME', value: storageAccountName }
            { name: 'AZURE_STORAGE_ACCOUNT_KEY', secretRef: 'storage-account-key' }
          ]
        }
      ]
    }
  }
}

// --- Ingest ---
// TODO: OTLP/gRPC (4317) はConsumptionプラン環境でのマルチポート公開 (additionalPortMappings) が
// ワークロードプロファイル環境限定のため、現状OTLP/HTTP (4318, FR-IN-01 Must) のみ公開している。
// FR-IN-02 (gRPC, Should) はワークロードプロファイル導入時の追従課題とする。
resource ingestApp 'Microsoft.App/containerApps@2024-03-01' = {
  name: 'ca-ingest-${resourceToken}'
  location: location
  tags: union(tags, { 'azd-service-name': 'ingest' })
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${managedIdentityId}': {}
    }
  }
  properties: {
    managedEnvironmentId: containerAppEnv.id
    configuration: {
      ingress: {
        external: true
        targetPort: 4318
        transport: 'auto'
      }
      secrets: [
        { name: 'clickhouse-password', keyVaultUrl: clickhousePasswordSecretUrl, identity: managedIdentityId }
      ]
    }
    template: {
      scale: {
        minReplicas: 0
        maxReplicas: 3
      }
      containers: [
        {
          name: 'ingest'
          image: ingestImage
          resources: {
            cpu: json('0.5')
            memory: '1Gi'
          }
          env: [
            { name: 'CLICKHOUSE_ENDPOINT', value: 'tcp://${clickhouseApp.properties.configuration.ingress.fqdn}:9000' }
            { name: 'CLICKHOUSE_USERNAME', value: 'default' }
            { name: 'CLICKHOUSE_PASSWORD', secretRef: 'clickhouse-password' }
            { name: 'CLICKHOUSE_DATABASE', value: 'default' }
            // 自己o11y (design.md 6.1)。Ingest自体はOTel Collector distributionなので、
            // 本来はこの接続文字列でCollectorのTelemetry設定 (service.telemetry) を有効化して
            // 自己メトリクス/ログをApp Insightsに送る想定。現状のotelcol-config.yamlはまだ
            // それを設定していない (follow-up)。
            { name: 'APPLICATIONINSIGHTS_CONNECTION_STRING', value: appInsightsConnectionString }
          ]
        }
      ]
    }
  }
}

// --- Query API ---
resource queryApp 'Microsoft.App/containerApps@2024-03-01' = {
  name: 'ca-query-${resourceToken}'
  location: location
  tags: union(tags, { 'azd-service-name': 'query' })
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${managedIdentityId}': {}
    }
  }
  properties: {
    managedEnvironmentId: containerAppEnv.id
    configuration: {
      ingress: {
        external: true
        targetPort: 8080
        transport: 'auto'
      }
      secrets: [
        { name: 'query-api-key', keyVaultUrl: queryApiKeySecretUrl, identity: managedIdentityId }
        // DSN はホスト名(ClickHouseの内部FQDN)とパスワードを含む合成値のため、
        // Key Vault参照ではなくBicep側で組み立てた値をそのままシークレットとして渡す。
        { name: 'clickhouse-dsn', value: 'clickhouse://default:${clickhousePassword}@${clickhouseApp.properties.configuration.ingress.fqdn}:9000/default' }
      ]
    }
    template: {
      scale: {
        minReplicas: 0
        maxReplicas: 3
      }
      containers: [
        {
          name: 'query'
          image: queryImage
          resources: {
            cpu: json('0.25')
            memory: '0.5Gi'
          }
          env: [
            { name: 'LISTEN_ADDR', value: ':8080' }
            { name: 'QUERY_API_KEY', secretRef: 'query-api-key' }
            { name: 'CLICKHOUSE_DSN', secretRef: 'clickhouse-dsn' }
            // 現状 apps/query のコードはこの変数をまだ読んでいない (follow-up: OTel SDKでの自己計装)。
            { name: 'APPLICATIONINSIGHTS_CONNECTION_STRING', value: appInsightsConnectionString }
          ]
        }
      ]
    }
  }
}

// --- MCP Server ---
resource mcpApp 'Microsoft.App/containerApps@2024-03-01' = {
  name: 'ca-mcp-${resourceToken}'
  location: location
  tags: union(tags, { 'azd-service-name': 'mcp' })
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${managedIdentityId}': {}
    }
  }
  properties: {
    managedEnvironmentId: containerAppEnv.id
    configuration: {
      ingress: {
        external: true
        targetPort: 8081
        transport: 'auto'
      }
      secrets: [
        { name: 'mcp-api-key', keyVaultUrl: mcpApiKeySecretUrl, identity: managedIdentityId }
        { name: 'query-api-key', keyVaultUrl: queryApiKeySecretUrl, identity: managedIdentityId }
      ]
    }
    template: {
      scale: {
        minReplicas: 0
        maxReplicas: 3
      }
      containers: [
        {
          name: 'mcp'
          image: mcpImage
          resources: {
            cpu: json('0.25')
            memory: '0.5Gi'
          }
          env: [
            { name: 'LISTEN_ADDR', value: ':8081' }
            { name: 'MCP_API_KEY', secretRef: 'mcp-api-key' }
            { name: 'QUERY_API_BASE_URL', value: 'https://${queryApp.properties.configuration.ingress.fqdn}' }
            { name: 'QUERY_API_KEY', secretRef: 'query-api-key' }
            // 現状 apps/mcp のコードはこの変数をまだ読んでいない (follow-up: OTel SDKでの自己計装)。
            { name: 'APPLICATIONINSIGHTS_CONNECTION_STRING', value: appInsightsConnectionString }
          ]
        }
      ]
    }
  }
}

output ingestOtlpHttpUrl string = 'https://${ingestApp.properties.configuration.ingress.fqdn}'
output queryApiUrl string = 'https://${queryApp.properties.configuration.ingress.fqdn}'
output mcpServerUrl string = 'https://${mcpApp.properties.configuration.ingress.fqdn}'
