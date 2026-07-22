import { Outlet } from "react-router-dom";

import AdminPanelBar from "../../components/admin/AdminPanelBar";
import LoginDialog from "@/components/Login";
import { AccountProvider, useAccount } from "@/contexts/AccountContext";
import { usePublicInfo } from "@/contexts/PublicInfoContext";
import { useIsMobile } from "@/hooks/use-mobile";
import { updateSettingsWithToast, useSettings } from "@/lib/api";
import { Button, Dialog } from "@radix-ui/themes";
import { useEffect, useState, type ReactNode } from "react";
import { Eula } from "@/utils/field";
import { normalizeLanguage, readStoredLanguage } from "@/utils/language";

const selectThemeBackground = (value: unknown) => {
  if (typeof value !== "string") return "";

  const variants = value.split("|").map((item) => item.trim());
  const darkVariant = variants[1] || variants[0] || "";

  return (
    darkVariant
      .split(",")
      .map((item) => item.trim())
      .find(Boolean) || ""
  );
};

const AdminLoginBackground = ({ children }: { children?: ReactNode }) => {
  const { publicInfo } = usePublicInfo();
  const isMobile = useIsMobile();
  const themeSettings = publicInfo?.theme_settings;
  const desktopBackground = selectThemeBackground(
    themeSettings?.backgroundImageUrlDesktop || themeSettings?.backgroundImage
  );
  const mobileBackground = selectThemeBackground(
    themeSettings?.backgroundImageUrlMobile ||
      themeSettings?.backgroundImageMobile
  );
  const background = isMobile
    ? mobileBackground || desktopBackground
    : desktopBackground;

  return (
    <main
      className={
        background
          ? "min-h-screen w-full bg-cover bg-center bg-fixed bg-no-repeat"
          : "min-h-screen w-full bg-accent-1"
      }
      style={{ backgroundImage: background ? `url(${background})` : "none" }}
    >
      {children}
    </main>
  );
};

const AuthenticatedAdminLayout = () => {
  const { settings, loading } = useSettings();
  const lang = readStoredLanguage() || "en";
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (loading) {
      setOpen(false);
    } else if (
      settings &&
      !settings.eula_accepted &&
      normalizeLanguage(lang).startsWith("zh")
    ) {
      setOpen(true);
    }
  }, [loading, settings, lang]);
  return (
    <>
      <Dialog.Root open={open}>
        <Dialog.Content>
          <Dialog.Content>
            <Dialog.Title>法律声明与合规指引</Dialog.Title>
            <div className="flex flex-col gap-2">
              <div className="max-h-[70vh] overflow-y-auto space-y-4">
                <pre className="text-wrap">{Eula}</pre>
              </div>
              <div className="flex flex-row gap-2 justify-end items-center">
                <Button
                  variant="soft"
                  color="red"
                  onClick={() => window.close()}
                >
                  不接受
                </Button>
                <Button
                  variant="solid"
                  onClick={() => {
                    setOpen(false);
                    updateSettingsWithToast(
                      { eula_accepted: true },
                      (key) => key
                    );
                  }}
                >
                  我已详细阅读并接受
                </Button>
              </div>
            </div>
          </Dialog.Content>
        </Dialog.Content>
      </Dialog.Root>
      <AdminPanelBar content={<Outlet />} />
    </>
  );
};

const AdminAccessGate = () => {
  const { account, loading } = useAccount();

  if (loading) {
    return <AdminLoginBackground />;
  }

  if (!account?.logged_in) {
    return (
      <AdminLoginBackground>
        <LoginDialog
          autoOpen
          hideTrigger
          preventClose
          showSettings={false}
          onLoginSuccess={() => window.location.reload()}
        />
      </AdminLoginBackground>
    );
  }

  return <AuthenticatedAdminLayout />;
};

const AdminLayout = () => (
  <AccountProvider>
    <AdminAccessGate />
  </AccountProvider>
);

export default AdminLayout;
