import { webview } from '@kit.ArkWeb';
import { http } from '@kit.NetworkKit';
import { hilog } from '@kit.PerformanceAnalysisKit';

const BACKEND_URL: string = 'http://127.0.0.1:28976/';
const TAG: string = 'HomeCast';

@Entry
@Component
struct Index {
  @State private pageUrl: string = '';
  @State private statusText: string = 'HomeCast 后端启动中…';
  private controller: webview.WebviewController = new webview.WebviewController();

  aboutToAppear(): void {
    this.waitBackend(0);
  }

  private async waitBackend(attempt: number): Promise<void> {
    const request = http.createHttp();
    try {
      const response = await request.request(BACKEND_URL, {
        method: http.RequestMethod.GET,
        connectTimeout: 1000,
        readTimeout: 1500,
        usingCache: false
      });
      if (response.responseCode === 200) {
        hilog.info(0x0000, 'HomeCast', `backend HTTP ready after ${attempt} retries`);
        this.pageUrl = BACKEND_URL;
        return;
      }
    } catch (err) {
      if (attempt % 10 === 0) {
        hilog.info(0x0000, 'HomeCast', `backend probe ${attempt}: ${JSON.stringify(err)}`);
      }
    } finally {
      request.destroy();
    }

    if (attempt < 100) {
      setTimeout(() => {
        this.waitBackend(attempt + 1);
      }, 200);
    } else {
      this.statusText = '后端未就绪，请查看 hilog';
      hilog.error(0x0000, 'HomeCast', 'backend did not become ready');
    }
  }

  build() {
    Column() {
      if (this.pageUrl.length > 0) {
        Web({ src: this.pageUrl, controller: this.controller })
          .javaScriptAccess(true)
          .domStorageAccess(true)
          .databaseAccess(true)
          .fileAccess(false)
          .width('100%')
          .height('100%')
          .onPageBegin((event) => {
            hilog.info(0x0000, 'HomeCast', `Web page begin: ${event.url}`);
          })
          .onPageEnd((event) => {
            hilog.info(0x0000, 'HomeCast', `Web page loaded: ${event.url}`);
          });
      } else {
        Text(this.statusText)
          .fontSize(16)
          .padding(24)
          .textAlign(TextAlign.Center)
          .width('100%')
          .height('100%');
      }
    }
    .width('100%')
    .height('100%');
  }
}
