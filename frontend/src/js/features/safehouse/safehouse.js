import "../../../css/app/community-sync.css";
import {
  ConnectToServer, GetCommunityServerStatuses, GetRootDirectory,
  PreviewServerMapSync, StartServerMapSync, RestoreLastServerMapSync,
  PreviewServerMapSyncTrash, ClearServerMapSyncTrash,
} from "../../../../wailsjs/go/app/App";
import { BrowserOpenURL } from "../../../../wailsjs/runtime/runtime";
import { getConfig, saveConfig } from "../../core/config.js";
import { switchAppPage } from "../../core/ui-shell.js";
import { mountSafehouseView } from "./safehouse-view.js";
import { showConfirmModal } from "../modals/confirm.js";

export function mountSafehousePage() {
  return mountSafehouseView(document.getElementById("safehouse-page-content"), {
    ConnectToServer, GetCommunityServerStatuses, GetRootDirectory,
    PreviewServerMapSync, StartServerMapSync, RestoreLastServerMapSync,
    PreviewServerMapSyncTrash, ClearServerMapSyncTrash, showConfirmModal,
    BrowserOpenURL, getConfig, saveConfig, switchAppPage,
  });
}
