# Multi-Project Cloud Run Slack Bot - Terraform Configuration

この設定は、複数のGCPプロジェクトを監視するCloud Run Slack Botを一つのホストプロジェクトにデプロイするためのTerraform構成です。

## 📋 前提条件

- Terraform >= 1.0
- Google Cloud SDK
- 適切なGCPプロジェクトとIAM権限
- Slack App（Bot Token、Signing Secret）

## 🏗️ アーキテクチャ

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Project 1     │    │   Project 2     │    │   Project 3     │
│   (Monitored)   │    │   (Monitored)   │    │   (Monitored)   │
├─────────────────┤    ├─────────────────┤    ├─────────────────┤
│ Cloud Run       │    │ Cloud Run       │    │ Cloud Run       │
│ Cloud Monitoring│    │ Cloud Monitoring│    │ Cloud Monitoring│
│ Pub/Sub Topic   │    │ Pub/Sub Topic   │    │ Pub/Sub Topic   │
│ Audit Logs      │    │ Audit Logs      │    │ Audit Logs      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────┐
                    │     Host Project        │
                    │                         │
                    │ ┌─────────────────────┐ │
                    │ │   Cloud Run Bot     │ │
                    │ │   (Multi-Project)   │ │
                    │ └─────────────────────┘ │
                    │                         │
                    │ ┌─────────────────────┐ │
                    │ │   Secret Manager    │ │
                    │ │   (Slack Secrets)   │ │
                    │ └─────────────────────┘ │
                    │                         │
                    │ ┌─────────────────────┐ │
                    │ │   Service Account   │ │
                    │ │   (Cross-Project)   │ │
                    │ └─────────────────────┘ │
                    └─────────────────────────┘
                                 │
                                 ▼
                         ┌─────────────┐
                         │   Slack     │
                         │   Channels  │
                         └─────────────┘
```

## 🚀 セットアップ手順

### 1. 設定ファイルの準備

```bash
# terraform.tfvarsファイルをコピー
cp terraform.tfvars.example terraform.tfvars

# 設定値を編集
vi terraform.tfvars
```

### 2. Terraform初期化

```bash
terraform init
```

### 3. プランの確認

```bash
terraform plan
```

### 4. デプロイ実行

```bash
terraform apply
```

## 📝 設定例

### terraform.tfvars

```hcl
# ホストプロジェクト（Botをデプロイする場所）
host_project_id = "my-bot-host-project"

# 監視対象プロジェクト
monitored_projects = [
  {
    project_id      = "web-frontend-project"
    region          = "us-central1"
    default_channel = "web-alerts"
    service_channels = {
      "web-app"     = "web-team"
      "api-gateway" = "api-team"
    }
  },
  {
    project_id      = "data-pipeline-project"
    region          = "us-east1"
    default_channel = "data-alerts"
    service_channels = {
      "etl-job"      = "data-team"
      "ml-inference" = "ml-team"
    }
  },
  {
    project_id      = "mobile-backend-project"
    region          = "europe-west1"
    default_channel = "mobile-alerts"
    service_channels = {
      "user-service"    = "mobile-team"
      "push-service"    = "mobile-team"
      "payment-service" = "payments-team"
    }
  }
]

# Slack設定
slack_bot_token      = "xoxb-your-bot-token"
slack_signing_secret = "your-signing-secret"
slack_app_mode       = "http"
default_channel      = "general"
```

### チャンネルマッピング結果

上記設定により、以下のチャンネルマッピングが自動生成されます：

- `web-alerts` → `web-frontend-project` (auto-detect enabled)
- `web-team` → `web-frontend-project` (auto-detect enabled)
- `api-team` → `web-frontend-project` (auto-detect enabled)
- `data-alerts` → `data-pipeline-project` (auto-detect enabled)
- `data-team` → `data-pipeline-project` (auto-detect enabled)
- `ml-team` → `data-pipeline-project` (auto-detect enabled)
- `mobile-alerts` → `mobile-backend-project` (auto-detect enabled)
- `mobile-team` → `mobile-backend-project` (auto-detect enabled)
- `payments-team` → `mobile-backend-project` (auto-detect enabled)

## 🔧 作成されるリソース

### ホストプロジェクト
- **Cloud Run Service**: Slack Bot本体
- **Service Account**: クロスプロジェクト権限付き
- **Secret Manager**: Slack認証情報
- **IAM Bindings**: 必要な権限設定

### 監視対象プロジェクト（各プロジェクト）
- **Pub/Sub Topic**: 監査ログ用
- **Pub/Sub Subscription**: Webhookプッシュ用
- **Logging Sink**: Cloud Run監査ログ
- **IAM Bindings**: Bot用アクセス権限

## 🔐 IAM権限

### ホストプロジェクト
- `roles/run.invoker`: Cloud Run呼び出し
- `roles/secretmanager.secretAccessor`: シークレット読み取り
- `roles/pubsub.subscriber`: Pub/Sub受信
- `roles/logging.logWriter`: ログ書き込み

### 監視対象プロジェクト
- `roles/run.viewer`: Cloud Runリソース読み取り
- `roles/monitoring.viewer`: メトリクス読み取り
- `roles/logging.viewer`: ログ読み取り

## 🌐 Slack App設定

デプロイ完了後、以下のURLをSlack Appに設定してください：

1. **Events API URL**: `https://your-bot-url/slack/events`
2. **Interactive Components URL**: `https://your-bot-url/slack/interaction`

## 📊 監視・運用

### ログ確認
```bash
# Cloud Runログ
gcloud run services logs read cloud-run-slack-bot --project=HOST_PROJECT_ID

# Pub/Sub監視
gcloud pubsub subscriptions list --project=MONITORED_PROJECT_ID
```

### メトリクス確認
- Cloud Run: Google Cloud Console > Cloud Run > メトリクス
- Pub/Sub: Google Cloud Console > Pub/Sub > 監視

## 🔄 更新・メンテナンス

### 監視プロジェクト追加
1. `terraform.tfvars`に新しいプロジェクトを追加
2. `terraform plan`で変更確認
3. `terraform apply`で適用

### 設定変更
```bash
# 設定変更後
terraform plan
terraform apply
```

## 🧹 クリーンアップ

```bash
# 全リソース削除
terraform destroy

# 特定のプロジェクトのみ削除
terraform destroy -target="google_pubsub_topic.audit_logs[\"project-id\"]"
```

## 🔍 トラブルシューティング

### よくある問題

1. **権限エラー**
   ```bash
   # サービスアカウントの権限確認
   gcloud projects get-iam-policy PROJECT_ID --flatten="bindings[].members" --format="table(bindings.role)" --filter="bindings.members:SERVICE_ACCOUNT_EMAIL"
   ```

2. **Pub/Sub接続エラー**
   ```bash
   # Pub/Subトピック確認
   gcloud pubsub topics list --project=PROJECT_ID

   # サブスクリプション確認
   gcloud pubsub subscriptions list --project=PROJECT_ID
   ```

3. **Bot応答なし**
   ```bash
   # Cloud Runログ確認
   gcloud run services logs read cloud-run-slack-bot --project=HOST_PROJECT_ID --limit=100
   ```

## 📚 参考資料

- [Cloud Run Slack Bot Documentation](../docs/multi-project-setup.md)
- [Terraform Google Provider](https://registry.terraform.io/providers/hashicorp/google/latest/docs)
- [Slack API Documentation](https://api.slack.com/)
