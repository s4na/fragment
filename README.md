# fragment

fragmentは、ローカルのドキュメントフォルダをCLI、MCP、Web UIから共通して読み書きするアプリケーションです。

プロジェクトごとに `flow` と `stock` のフォルダを持ち、自由な内容のMarkdownを保存します。チャットなどをflowに残し、AIとの対話で保存を指示した内容をstockに溜めていきます。

ghqのリポジトリやブランチを切り替えても同じ保存先を参照し、Goの共通コアで操作します。PreactのWeb UIでは、保存されたMarkdownを整形して閲覧できます。

## ビルドと検証

macOSのローカルディスクを対象に検証しています。開発にはGo 1.26.3以降、Node.js 22.12以降、pnpm、Socket Firewallの `sfw` が必要です。ビルド済みの実行ファイルは、Node.jsやpnpmなしで利用できます。

```sh
make build
make test
```

`make build` はPreactの画面をビルドし、埋め込んだ実行ファイルを `bin/fragment` に作成します。フロントエンドのpnpmコマンドは `sfw` 経由で実行します。`make test` は画面のビルド、Goのrace検出付きテスト、`go vet` を実行します。

## 使い始める

```sh
bin/fragment config init "$HOME/Documents/fragment"
bin/fragment project create "新サービス"
printf '# 概要\n\n自由なMarkdown本文。\n' | bin/fragment doc create \
  --project "新サービス" --kind stock --path "概要.md"
bin/fragment serve
```

`serve` が表示するURLをブラウザで開くと接続できます。最初の接続で起動時の一回限りのトークンを使い、その後はローカルのセッションCookieで認証します。別のブラウザから接続する場合はサーバーを再起動して新しいURLを使ってください。

保存先の名前は自由です。設定を同じOSユーザーで共有するため、リポジトリやブランチを切り替えても同じデータを読み書きします。設定を変更したら常駐しているMCPとWebサーバーを再起動してください。`config init` は保存先の設定を変更しますが、既存の文書を移動しません。

## CLIとMCP

CLIはJSONを標準出力へ返します。本文は標準入力、または `--file` で指定したファイルから入力できます。

```sh
bin/fragment project list
bin/fragment doc list --project "新サービス" --kind flow
bin/fragment doc get --project "新サービス" --kind stock --path "概要.md"
bin/fragment doc update --project "新サービス" --kind stock --path "概要.md" \
  --revision "取得結果のrevision" --file ./updated.md
bin/fragment mcp
```

AIクライアントには、ビルド済みの実行ファイルの絶対パスと引数 `mcp` を登録してください。stdioで `list_projects`、`create_project`、`list_documents`、`get_document`、`create_document`、`update_document` を提供します。

```json
{
  "mcpServers": {
    "fragment": {
      "command": "/absolute/path/to/fragment/bin/fragment",
      "args": ["mcp"]
    }
  }
}
```

stockへの保存や更新は利用者の指示に従います。アプリケーション自体は会話を自動取得したり、本文の構成を決めたりしません。競合した場合は最新の本文を取得し、変更内容を検討してから新しいリビジョンで更新してください。

## 初期版の範囲

文書の作成・取得・更新、一覧のページング、Markdownの表示・編集・プレビューを提供します。保存先の `.fragment/write.lock` にOSの排他ロックを取得し、ロック内で取得時の内容ハッシュを比較します。文書は一時ファイルから置き換え、古い版による上書きを拒否します。

- 文書の上限は8MiB、一覧は一回100件（最大1000件）、書き込みロックの待機は5秒です。
- Webサーバーはloopbackのみを使用します。生HTMLは表示せず、HTMLをサニタイズします。
- flowとstock内の相対Markdownリンクを解決します。画像配信やMermaid表示、改名、移動、削除、同期、編集履歴は未対応です。
- 安全な同時書き込みの範囲はアプリケーションの入口間です。外部エディターや同期ソフトはOSロックに従わない場合があります。

通常の設定は `os.UserConfigDir()` 配下の `fragment/config.json` にあります。検証用には絶対パスの `FRAGMENT_CONFIG_DIR` で設定ディレクトリを切り替えられます。CLI・MCP・Webサーバーには同じ値を渡してください。利用者の文書とアプリケーションのソースコードは別の場所に保管します。

- [共通ドキュメントストアのデザインドック](docs/design-doc.md)
