export function BrandLogo({ className = "" }: { className?: string }) {
  return (
    <img
      className={"brand-logo " + className}
      src="/assets/ane-logo.png"
      alt="ANE"
      width={898}
      height={339}
    />
  );
}
