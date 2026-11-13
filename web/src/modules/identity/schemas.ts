import { z } from "zod";
import { code, text } from "../../shared/lib/validation";
export const passwordBytes = (value: string) =>
  new TextEncoder().encode(value).length;
export const loginSchema = z.object({
  email: z.string().email("Введите email"),
  password: z.string().min(1, "Введите пароль"),
});
export const profileSchema = z.object({
  display_name: text(80),
  preferred_categories: z.array(code).max(5),
});
export const registerSchema = loginSchema.extend({
  ...profileSchema.shape,
  password: z
    .string()
    .refine(
      (s) => passwordBytes(s) >= 12 && passwordBytes(s) <= 72,
      "Пароль должен занимать от 12 до 72 байт (латиница: 12–72 символа)",
    ),
});
