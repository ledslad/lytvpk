import "../../../css/app/community-sync.css";
import {
  ConnectToServer, GetCommunityServerStatuses, GetRootDirectory,
  PreviewServerMapSync, StartServerMapSync, RestoreLastServerMapSync,
} from "../../../../wailsjs/go/app/App";
import { BrowserOpenURL } from "../../../../wailsjs/runtime/runtime";
import { getConfig, saveConfig } from "../../core/config.js";
import { switchAppPage } from "../../core/ui-shell.js";
import { mountSafehouseView } from "./safehouse-view.js";

export function mountSafehousePage() {
  return mountSafehouseView(document.getElementById("safehouse-page-content"), {
    ConnectToServer, GetCommunityServerStatuses, GetRootDirectory,
    PreviewServerMapSync, StartServerMapSync, RestoreLastServerMapSync,
    BrowserOpenURL, getConfig, saveConfig, switchAppPage,
  });
}
