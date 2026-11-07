import { Suspense } from "react";
import { ReviewScreen } from "./review-screen";

async function Loader({ params }: { params: PageProps<"/artifacts/[id]/review">["params"] }) {
  const { id } = await params;
  return <ReviewScreen id={id} />;
}

export default function MockupReviewPage({ params }: PageProps<"/artifacts/[id]/review">) {
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
