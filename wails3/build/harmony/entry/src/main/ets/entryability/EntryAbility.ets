import { AbilityConstant, UIAbility, Want } from '@kit.AbilityKit';
import { hilog } from '@kit.PerformanceAnalysisKit';
import { window } from '@kit.ArkUI';
import { startServer } from 'libentry.so';

export default class EntryAbility extends UIAbility {
  onCreate(want: Want, launchParam: AbilityConstant.LaunchParam): void {
    try {
      // Go 后端只接收正式 App 沙盒目录；业务与前端仍全部由 Go 提供。
      const dataDir: string = this.context.filesDir;
      hilog.info(0x0000, 'HomeCast', `Go backend start requested: ${dataDir}`);
      const ok: boolean = startServer(dataDir);
      hilog.info(0x0000, 'HomeCast', `Go backend start returned: ${ok}`);
    } catch (err) {
      hilog.error(0x0000, 'HomeCast', `Go backend start failed: ${String(err)} ${JSON.stringify(err)}`);
    }
  }

  onWindowStageCreate(windowStage: window.WindowStage): void {
    windowStage.loadContent('pages/Index', (err) => {
      if (err.code) {
        hilog.error(0x0000, 'HomeCast', `loadContent failed: ${JSON.stringify(err)}`);
        return;
      }
      hilog.info(0x0000, 'HomeCast', 'loadContent pages/Index succeeded');
    });
  }
}
