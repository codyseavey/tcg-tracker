/// Card condition codes and labels used throughout the app.
/// These match the standard TCG grading conditions.
class CardConditions {
  CardConditions._();

  /// Standard condition codes in order from best to worst
  static const List<String> codes = ['M', 'NM', 'LP', 'MP', 'HP', 'D'];

  /// Human-readable labels for each condition code
  static const Map<String, String> labels = {
    'M': 'Mint',
    'NM': 'Near Mint',
    'LP': 'Lightly Played',
    'MP': 'Moderately Played',
    'HP': 'Heavily Played',
    'D': 'Damaged',
  };

  /// Get the label for a condition code, with fallback
  static String getLabel(String code) => labels[code] ?? code;
}
