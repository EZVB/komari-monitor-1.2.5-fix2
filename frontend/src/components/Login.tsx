import * as React from "react";
import {
  Button,
  Dialog,
  Flex,
  IconButton,
  Text,
  TextField,
} from "@radix-ui/themes";
import { useTranslation } from "react-i18next";

import { AccountProvider, useAccount } from "@/contexts/AccountContext";
import { TablerSettings } from "./Icones/Tabler";

type LoginDialogProps = {
  trigger?: React.ReactNode | string;
  autoOpen?: boolean;
  showSettings?: boolean;
  info?: string | React.ReactNode;
  onLoginSuccess?: () => void;
  hideTrigger?: boolean;
  preventClose?: boolean;
};

const LoginDialog = ({
  trigger,
  autoOpen = false,
  showSettings = true,
  info,
  onLoginSuccess,
  hideTrigger = false,
  preventClose = false,
}: LoginDialogProps) => {
  const InnerLayout = () => {
    const { account, loading, error, refresh } = useAccount();
    const [t] = useTranslation();
    const [username, setUsername] = React.useState("");
    const [password, setPassword] = React.useState("");
    const [twoFac, setTwoFac] = React.useState("");
    const [errorMsg, setErrorMsg] = React.useState("");
    const [isLoading, setIsLoading] = React.useState(false);
    const [require2FA, setRequire2FA] = React.useState(false);
    const [open, setOpen] = React.useState(autoOpen);
    const isFormValid =
      username.trim() !== "" &&
      password.trim() !== "" &&
      (!require2FA || twoFac.trim() !== "");

    React.useEffect(() => {
      if (autoOpen) setOpen(true);
    }, [autoOpen]);

    const handleLogin = async () => {
      if (!isFormValid) {
        setErrorMsg("Username and password are required");
        return;
      }

      setErrorMsg("");
      setIsLoading(true);
      try {
        const response = await fetch("/api/login", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            username,
            password,
            ...(twoFac ? { "2fa_code": twoFac } : {}),
          }),
        });
        const data = await response.json();
        if (response.ok) {
          refresh();
          if (onLoginSuccess) {
            onLoginSuccess();
            return;
          }
          window.open("/admin", "_self");
          return;
        }
        if (data.message === "2FA code is required") {
          setRequire2FA(true);
          return;
        }
        setErrorMsg(data.message || "Login failed");
      } catch (loginError) {
        setErrorMsg("Network error");
        console.error(loginError);
      } finally {
        setIsLoading(false);
      }
    };

    if (loading) return <Button disabled>{t("loading")}</Button>;
    if (error || !account) {
      return (
        <Button disabled color="red">
          Error
        </Button>
      );
    }
    if (account.logged_in) {
      if (!showSettings) return null;
      return (
        <a href="/admin" target="_blank">
          <IconButton>
            <TablerSettings />
          </IconButton>
        </a>
      );
    }

    return (
      <Dialog.Root
        open={open}
        onOpenChange={(nextOpen) => {
          if (!preventClose || nextOpen) setOpen(nextOpen);
        }}
      >
        {!hideTrigger && (
          <Dialog.Trigger>
            {trigger ?? <Button>{t("login.title")}</Button>}
          </Dialog.Trigger>
        )}
        <Dialog.Content maxWidth="450px">
          <Dialog.Title>{t("login.title")}</Dialog.Title>
          <Dialog.Description size="2" mb="4">
            <span className="flex flex-col justify-center gap-2">
              <span>{t("login.desc")}</span>
              {info && <span>{info}</span>}
            </span>
          </Dialog.Description>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (isFormValid && !isLoading) void handleLogin();
            }}
          >
            <Flex direction="column" gap="3">
              <label>
                <Text as="div" size="2" mb="1" weight="bold">
                  {t("login.username")}
                </Text>
                <TextField.Root
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  placeholder="admin"
                  disabled={isLoading}
                  autoComplete="username"
                  autoFocus
                />
              </label>
              <label>
                <Text as="div" size="2" mb="1" weight="bold">
                  {t("login.password")}
                </Text>
                <TextField.Root
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  type="password"
                  placeholder={t("login.password_placeholder")}
                  disabled={isLoading}
                  autoComplete="current-password"
                />
              </label>
              {require2FA && (
                <label>
                  <Text as="div" size="2" mb="1" weight="bold">
                    {t("login.two_factor")}
                  </Text>
                  <TextField.Root
                    value={twoFac}
                    onChange={(event) => setTwoFac(event.target.value)}
                    placeholder="000000"
                    disabled={isLoading}
                    autoComplete="one-time-code"
                    inputMode="numeric"
                  />
                </label>
              )}
              {errorMsg && (
                <Text as="div" size="2" color="red">
                  {errorMsg}
                </Text>
              )}
              <Button type="submit" disabled={isLoading || !isFormValid}>
                {isLoading ? "Logging in..." : t("login.title")}
              </Button>
            </Flex>
          </form>
        </Dialog.Content>
      </Dialog.Root>
    );
  };

  return (
    <AccountProvider>
      <InnerLayout />
    </AccountProvider>
  );
};

export default LoginDialog;
