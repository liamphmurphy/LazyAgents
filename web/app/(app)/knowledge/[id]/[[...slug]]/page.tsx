import { PageScreen } from "../../_components/page-screen";

export default async function Page({ params }: PageProps<"/knowledge/[id]/[[...slug]]">) {
  const { id } = await params;
  return <PageScreen id={id} />;
}
