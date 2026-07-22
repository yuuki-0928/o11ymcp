# セキュリティポリシー

## サポート対象バージョン

個人開発の小規模OSSプロジェクトのため、`main` ブランチの最新版のみをサポートします。
過去バージョンへのバックポートは行いません。

## 脆弱性の報告

セキュリティ上の問題を発見した場合は、**通常のGitHub Issueには公開で登録しないでください**
(攻撃者に悪用の手がかりを与えてしまうため)。代わりに以下のいずれかの方法で報告してください。

1. このリポジトリの GitHub Security Advisories ([Report a vulnerability](../../security/advisories/new)) から報告する (推奨)
2. yuki.development.project@gmail.com へ直接メールする

報告には、再現手順・影響範囲・想定される深刻度を可能な範囲で含めてください。

## 対応方針

- 報告受領後、可能な限り速やかに一次返信します (個人開発のため対応時間の保証はできません)。
- 深刻度に応じて修正版をリリースし、[GitHub Security Advisories](../../security/advisories) で公表します。
- FR-MON-01 (依存パッケージの脆弱性自動検知) により、サードパーティ依存の既知脆弱性は
  Dependabot 経由でも継続的に検知しています ([`.github/dependabot.yml`](.github/dependabot.yml))。
