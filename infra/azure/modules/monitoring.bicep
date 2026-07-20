// 監視モジュール (FR-OP-03)。自己o11y (要件定義書 6.1: Azure版はApplication Insights Basic)。
// Log Analytics WorkspaceはContainer Apps環境のログ出力先としても兼用する。
param location string
param resourceToken string
param tags object

resource logAnalytics 'Microsoft.OperationalInsights/workspaces@2023-09-01' = {
  name: 'log-${resourceToken}'
  location: location
  tags: tags
  properties: {
    sku: {
      name: 'PerGB2018'
    }
    // NFR-C-01: 無償枠/低コストを優先。保持期間を最小 (既定30日) にして課金を抑える。
    retentionInDays: 30
  }
}

resource appInsights 'Microsoft.Insights/components@2020-02-02' = {
  name: 'appi-${resourceToken}'
  location: location
  tags: tags
  kind: 'web'
  properties: {
    Application_Type: 'web'
    WorkspaceResourceId: logAnalytics.id
    // Basic相当: サンプリングやアラートルールは追加せず、必要最小限の構成にとどめる。
  }
}

output logAnalyticsWorkspaceId string = logAnalytics.id
output appInsightsConnectionString string = appInsights.properties.ConnectionString
