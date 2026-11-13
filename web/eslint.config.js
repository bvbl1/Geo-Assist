import tseslint from "typescript-eslint";
import hooks from "eslint-plugin-react-hooks";
export default tseslint.config(
  { ignores: ["src/shared/api/generated/**"] },
  ...tseslint.configs.recommended,
  {
    plugins: { "react-hooks": hooks },
    rules: {
      "react-hooks/rules-of-hooks": "error",
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_" },
      ],
    },
  },
);
