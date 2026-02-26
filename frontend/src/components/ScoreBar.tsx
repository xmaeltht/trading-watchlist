interface Props {
  label: string;
  score: number;
  weight?: string;
}

export default function ScoreBar({ label, score, weight }: Props) {
  const color = score >= 70 ? "bg-green-500" : score >= 40 ? "bg-yellow-500" : "bg-red-500";
  return (
    <div className="space-y-1">
      <div className="flex justify-between text-xs text-gray-400">
        <span>{label} {weight && <span className="text-gray-600">({weight})</span>}</span>
        <span className="font-mono font-bold text-white">{score.toFixed(0)}</span>
      </div>
      <div className="h-1.5 bg-gray-800 rounded-full overflow-hidden">
        <div className={`h-full rounded-full transition-all ${color}`} style={{ width: `${score}%` }} />
      </div>
    </div>
  );
}
