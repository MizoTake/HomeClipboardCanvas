# 検証結果

2026-10-07、Windowsの開発PCで検証しました。プロセスはloopbackだけで起動しています。

## 自動検証

| 検証 | 結果・対象 |
| --- | --- |
| `scripts/build.ps1` | npm ci、型検証、Vite、Go test・vet、Windows EXE生成 |
| Gitソースからの再ビルド | mainのgit archiveをtools/clean-checkoutへ展開。生成済みWeb UIやnode_modulesなしの状態でbuild.ps1が成功し、そのEXEを最終distへ配置 |
| Goテスト | 22テスト成功。TTL60分の境界、作成・移動・revision競合・削除・全削除、画像の独立保存・期限切れ解放、メモリ・個数・サイズ制限、同時作成、遅い接続からの再接続 |
| セキュリティテスト | LAN CIDR、IPv4-mapped IPv6、追加CIDR、Forwardedヘッダーの無視、認証、期限切れ・偽造Cookie、再起動でのセッション失効、Origin拒否、requestサイズ、rate limit、TLS Secure Cookieと平文HTTP拒否 |
| Portable設定 | 相対証明書パスの解決、未知フィールド拒否、開発LAN bind拒否、証明書必須 |
| Startupテスト | 一時ディレクトリ内に実際の.lnk生成、日本語・引用符を含むパス、重複登録防止、所有するショートカットの解除 |
| Playwright | 22成功、2件は端末別の対象外としてスキップ（デスクトップのタッチ・モバイルのマウス。対応する操作は別ケースで成功）。デスクトップ・Pixel 7相当のChromium、貼り付け、画像優先、HTML無解釈、元画像バイト一致、実際のClipboard API、コピー非対応の案内、複数接続同期、カード操作、パン・ズーム、長押し、再接続、URLトークン除去、表示位置の独立、期限切れ通知、遅延応答でのカード復活防止 |
| `scripts/acceptance.mjs` | EXEだけを隔離フォルダから起動。終了・再起動で画像とテキストが復元されない。テスト用1分TTLで90秒後にWebSocket通知からカードが消えた。ブラウザpageerror 0件 |
| 実EXEの拒否確認 | 証明書なしの通常起動を拒否。dev-httpで0.0.0.0へのbindを拒否 |
| `status` | 開発PCのstartupRegistered=false |

Goテストの初期失敗を確認した後にStore・HTTP・設定・Startup機能を実装しています。ブラウザテストで、offline状態の通知、同一フレームのtouchmove/touchend、HTTP変更応答と削除通知の順序問題を再現し、修正を検証しました。テストの高速連続ログインでrate limitに当たったため、セッションをテスト開始時に一度だけ作り、通常APIの補充レートを尊重してケース間を空けています。製品側の制限は無効化していません。

期限切れ通知の短いUIテストではWebSocket通知を注入します。別のAcceptanceでは、時間を進めるモックを使わず、実際のEXEの期限切れ処理を待って確認しています。Clipboard APIは通常のChromium権限を付与する実テストと、非対応状態を模擬するテストの両方を実行します。

## 検証していない範囲

開発PCの制約に従い、実LANへの公開、別の物理端末からの接続、本番証明書の信頼設定、実Startup登録やWindowsログイン時の起動は実施していません。スマートフォンはChromiumのエミュレーションであり、実機Safari・Androidの権限UIは未検証です。

実運用前にはサーバーPCで証明書と接続元ネットワークを設定し、利用するPC・スマートフォンから貼り付け・コピーを確認してください。画像コピーのブラウザ互換性には「開く」「保存」のフォールバックがあります。

Goのrace detectorは、この環境にCGo/Cコンパイラがないため実行できませんでした。`go test -race` は `-race requires cgo` で終了しています。通常のGoテストには同時作成と制限の検証を含みます。

## 成果物とログ

配布用EXEは `dist/HomeClipboardCanvas.exe`。画面は `artifacts/desktop-empty.png` と `artifacts/mobile-empty.png`、実時間検証は `artifacts/acceptance.json`。PlaywrightのHTMLレポートは `frontend/playwright-report/index.html`。生成物、ブラウザ、Goツールチェーン、設定・シークレットはGitに含めません。
