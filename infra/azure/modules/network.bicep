// ネットワークモジュール (FR-OP-03: 役割ごとのBicepモジュール分割)。
// Container Apps 環境をVNet統合し、将来的にプライベートエンドポイント等を
// 追加しやすい土台にしておく。現時点ではNAT Gatewayやファイアウォールは
// 置かず、追加コストが発生しない最小構成 (NFR-C-01) にとどめている。
param location string
param resourceToken string
param tags object

resource vnet 'Microsoft.Network/virtualNetworks@2024-05-01' = {
  name: 'vnet-${resourceToken}'
  location: location
  tags: tags
  properties: {
    addressSpace: {
      addressPrefixes: ['10.0.0.0/16']
    }
    subnets: [
      {
        name: 'snet-container-apps'
        properties: {
          // Container Apps環境 (Consumptionプラン) がVNet統合に要求する最小 /23 を確保。
          addressPrefix: '10.0.0.0/23'
          delegations: [
            {
              name: 'Microsoft.App.environments'
              properties: {
                serviceName: 'Microsoft.App/environments'
              }
            }
          ]
        }
      }
    ]
  }
}

output containerAppsSubnetId string = vnet.properties.subnets[0].id
