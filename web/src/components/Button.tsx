import { Button as HeroButton, Spinner, type ButtonProps } from "@heroui/react";

/** HeroUI's Button, but busy looks busy: while isPending it shows a spinner in front of its label. */
export function Button({ children, isPending, ...props }: ButtonProps) {
  return (
    <HeroButton isPending={isPending} {...props}>
      {(state) => (
        <>
          {isPending ? <Spinner size="sm" color="current" /> : null}
          {typeof children === "function" ? children(state) : children}
        </>
      )}
    </HeroButton>
  );
}
