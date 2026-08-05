/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Link, useSearch } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useStatus } from "@/hooks/use-status";
import { AuthLayout } from "../auth-layout";
import { TermsFooter } from "../components/terms-footer";
import { UserAuthForm } from "./components/user-auth-form";

export function SignIn() {
  const { t } = useTranslation();
  const { redirect } = useSearch({ from: "/(auth)/sign-in" });
  const { status } = useStatus();

  return (
    <AuthLayout>
      {/* Background gradient orbs */}
      <div className="pointer-events-none fixed inset-0 -z-10 overflow-hidden">
        <div
          className="absolute -left-[10%] -top-[20%] h-[60vw] w-[60vw] rounded-full opacity-60 blur-[80px]"
          style={{
            background:
              "radial-gradient(circle, rgba(0, 229, 255, 0.22) 0%, rgba(0, 229, 255, 0.08) 40%, transparent 70%)",
          }}
        />
        <div
          className="absolute -right-[15%] top-[15%] h-[55vw] w-[55vw] rounded-full opacity-60 blur-[80px]"
          style={{
            background:
              "radial-gradient(circle, rgba(139, 92, 246, 0.18) 0%, rgba(139, 92, 246, 0.06) 40%, transparent 70%)",
          }}
        />
        <div
          className="absolute -bottom-[10%] left-[20%] h-[45vw] w-[45vw] rounded-full opacity-60 blur-[80px]"
          style={{
            background:
              "radial-gradient(circle, rgba(247, 37, 133, 0.15) 0%, rgba(247, 37, 133, 0.05) 40%, transparent 70%)",
          }}
        />
      </div>

      <div className="w-full space-y-8">
        {/* Badge */}
        <div className="flex justify-center">
          <div
            className="inline-flex items-center gap-2.5 rounded-full px-6 py-3 text-sm font-semibold"
            style={{
              background:
                "linear-gradient(135deg, rgba(255, 255, 255, 0.95) 0%, rgba(255, 255, 255, 0.75) 50%, rgba(255, 255, 255, 0.85) 100%)",
              border: "1px solid rgba(255, 255, 255, 1)",
              borderRightColor: "rgba(255, 255, 255, 0.6)",
              borderBottomColor: "rgba(255, 255, 255, 0.5)",
              backdropFilter: "blur(60px) saturate(250%) brightness(1.05)",
              color: "rgba(26, 26, 46, 0.75)",
              letterSpacing: "0.02em",
              boxShadow:
                "0 2px 4px 0 rgba(255, 255, 255, 1) inset, 0 8px 32px rgba(0, 0, 0, 0.12), 0 2px 8px rgba(0, 0, 0, 0.08)",
            }}
          >
            <span
              className="inline-block h-[7px] w-[7px] rounded-full"
              style={{
                background: "linear-gradient(135deg, #00e5ff, #8b5cf6)",
              }}
            />
            智象API · 企业级中转
          </div>
        </div>

        {/* Title */}
        <div className="space-y-4">
          <h1
            className="text-center text-5xl font-black tracking-[-0.06em] sm:text-6xl"
            style={{
              background:
                "linear-gradient(135deg, #00b4d8 0%, #8b5cf6 50%, #f72585 100%)",
              WebkitBackgroundClip: "text",
              WebkitTextFillColor: "transparent",
              backgroundClip: "text",
            }}
          >
            {t("Sign in")}
          </h1>
          {!status?.self_use_mode_enabled && (
            <p
              className="text-center text-base font-medium"
              style={{ color: "rgba(26, 26, 46, 0.55)" }}
            >
              {t("Don't have an account?")}{" "}
              <Link
                to="/sign-up"
                className="font-semibold transition-opacity hover:opacity-75"
                style={{
                  background:
                    "linear-gradient(135deg, #00b4d8 0%, #8b5cf6 50%, #f72585 100%)",
                  WebkitBackgroundClip: "text",
                  WebkitTextFillColor: "transparent",
                  backgroundClip: "text",
                }}
              >
                {t("Sign up")}
              </Link>
            </p>
          )}
        </div>

        {/* Form Card */}
        <div
          className="mx-auto max-w-md space-y-6 rounded-3xl p-8"
          style={{
            background:
              "linear-gradient(135deg, rgba(255, 255, 255, 0.85) 0%, rgba(255, 255, 255, 0.5) 50%, rgba(255, 255, 255, 0.7) 100%)",
            border: "1px solid rgba(255, 255, 255, 1)",
            borderRightColor: "rgba(255, 255, 255, 0.5)",
            borderBottomColor: "rgba(255, 255, 255, 0.4)",
            backdropFilter: "blur(50px) saturate(240%) brightness(1.05)",
            boxShadow:
              "0 2px 4px 0 rgba(255, 255, 255, 1) inset, 0 12px 48px rgba(0, 0, 0, 0.12), 0 4px 16px rgba(0, 0, 0, 0.08)",
          }}
        >
          <UserAuthForm redirectTo={redirect} />
        </div>

        <TermsFooter
          variant="sign-in"
          status={status}
          className="text-center"
          style={{ color: "rgba(26, 26, 46, 0.4)" }}
        />
      </div>
    </AuthLayout>
  );
}
